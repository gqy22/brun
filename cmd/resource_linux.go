//go:build linux

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ResourceUsage struct {
	PeakRSSKB int64
	CPUTimeMs int64
}

func ResourceSupported() bool { return true }

type ProcessInfo struct {
	PID        int    `json:"pid"`
	PPID       int    `json:"ppid"`
	PGID       int    `json:"pgid"`
	Depth      int    `json:"depth"`
	Role       string `json:"role"`
	State      string `json:"state"`
	IsActive   bool   `json:"is_active"`
	Comm       string `json:"comm"`
	RSSKB      int64  `json:"rss_kb"`
	CPUTime    int64  `json:"cpu_time_ms"`
	CPUDeltaMs int64  `json:"cpu_delta_ms"`
	Cmdline    string `json:"cmdline"`
}

type procStat struct {
	pid        int
	pgrp       int
	utimeTicks uint64
	stimeTicks uint64
}

type procStatFull struct {
	pid        int
	ppid       int
	pgrp       int
	state      string
	comm       string
	utimeTicks uint64
	stimeTicks uint64
}

type processTreeNode struct {
	pid   int
	depth int
}

type ProcessGroupSampler struct {
	pgid     int
	interval time.Duration
	done     chan struct{}
	stopped  chan ResourceUsage
	once     sync.Once
	stopMu   sync.Mutex

	mu            sync.Mutex
	usage         ResourceUsage
	stoppedUsage  ResourceUsage
	stoppedIsRead bool
}

func StartProcessGroupSampler(pgid int, interval time.Duration) *ProcessGroupSampler {
	if interval <= 0 {
		interval = time.Second
	}

	s := &ProcessGroupSampler{
		pgid:     pgid,
		interval: interval,
		done:     make(chan struct{}),
		stopped:  make(chan ResourceUsage, 1),
	}
	s.record(sampleProcessGroup(pgid))

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.record(sampleProcessGroup(pgid))
			case <-s.done:
				s.record(sampleProcessGroup(pgid))
				s.stopped <- s.snapshot()
				return
			}
		}
	}()

	return s
}

func (s *ProcessGroupSampler) Stop() ResourceUsage {
	if s == nil {
		return ResourceUsage{}
	}

	s.stopMu.Lock()
	defer s.stopMu.Unlock()
	if s.stoppedIsRead {
		return s.stoppedUsage
	}

	s.once.Do(func() {
		close(s.done)
	})

	s.stoppedUsage = <-s.stopped
	s.stoppedIsRead = true
	return s.stoppedUsage
}

func (s *ProcessGroupSampler) record(usage ResourceUsage) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if usage.PeakRSSKB > s.usage.PeakRSSKB {
		s.usage.PeakRSSKB = usage.PeakRSSKB
	}
	if usage.CPUTimeMs > s.usage.CPUTimeMs {
		s.usage.CPUTimeMs = usage.CPUTimeMs
	}
}

func (s *ProcessGroupSampler) snapshot() ResourceUsage {
	if s == nil {
		return ResourceUsage{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}

func sampleProcessGroup(pgid int) ResourceUsage {
	return sampleProcessGroupFromProc("/proc", pgid, clockTicksPerSecond())
}

func ListProcessGroup(pgid int) []ProcessInfo {
	return listProcessGroupFromProc("/proc", pgid, clockTicksPerSecond())
}

func ListProcessTree(rootPID int) []ProcessInfo {
	return listProcessTreeFromProc("/proc", rootPID, clockTicksPerSecond())
}

func ListProcessTreeWithActivity(rootPID int, interval time.Duration) []ProcessInfo {
	before := ListProcessTree(rootPID)
	if len(before) == 0 {
		return nil
	}
	if interval > 0 {
		time.Sleep(interval)
	}
	after := ListProcessTree(rootPID)
	if len(after) == 0 {
		return markProcessActivity(nil, before)
	}
	return markProcessActivity(before, after)
}

func listProcessGroupFromProc(procRoot string, pgid int, ticksPerSecond int64) []ProcessInfo {
	if pgid <= 0 || ticksPerSecond <= 0 {
		return nil
	}

	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}

	var procs []ProcessInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}

		dir := filepath.Join(procRoot, entry.Name())
		statData, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			continue
		}
		ps, err := parseProcStatFull(statData)
		if err != nil || ps.pgrp != pgid {
			continue
		}

		var rssKB int64
		if statusData, err := os.ReadFile(filepath.Join(dir, "status")); err == nil {
			rssKB = readStatusValueKB(statusData, "VmRSS:")
		}

		cmdline := ""
		if clData, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			cmdline = strings.ReplaceAll(string(clData), "\x00", " ")
			cmdline = strings.TrimSpace(cmdline)
		}

		cpuMs := int64(ps.utimeTicks+ps.stimeTicks) * 1000 / ticksPerSecond

		info := ProcessInfo{
			PID:      ps.pid,
			PPID:     ps.ppid,
			PGID:     ps.pgrp,
			Depth:    0,
			Role:     "process",
			State:    ps.state,
			IsActive: isActiveProcessState(ps.state),
			Comm:     ps.comm,
			RSSKB:    rssKB,
			CPUTime:  cpuMs,
			Cmdline:  cmdline,
		}
		procs = append(procs, info)
	}
	return procs
}

func sampleProcessGroupFromProc(procRoot string, pgid int, ticksPerSecond int64) ResourceUsage {
	if pgid <= 0 || ticksPerSecond <= 0 {
		return ResourceUsage{}
	}

	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return ResourceUsage{}
	}

	var rssKB int64
	var cpuTicks uint64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}

		dir := filepath.Join(procRoot, entry.Name())
		statData, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			continue
		}
		stat, err := parseProcStat(statData)
		if err != nil || stat.pgrp != pgid {
			continue
		}

		cpuTicks += stat.utimeTicks + stat.stimeTicks
		if statusData, err := os.ReadFile(filepath.Join(dir, "status")); err == nil {
			rssKB += readStatusValueKB(statusData, "VmRSS:")
		}
	}

	return ResourceUsage{
		PeakRSSKB: rssKB,
		CPUTimeMs: int64(cpuTicks) * 1000 / ticksPerSecond,
	}
}

func SampleProcessTree(rootPID int) ResourceUsage {
	return sampleProcessTreeFromProc("/proc", rootPID, clockTicksPerSecond())
}

func listProcessTreeFromProc(procRoot string, rootPID int, ticksPerSecond int64) []ProcessInfo {
	if rootPID <= 0 || ticksPerSecond <= 0 {
		return nil
	}

	nodes := walkProcessTree(procRoot, rootPID)
	if len(nodes) == 0 {
		return nil
	}

	procs := make([]ProcessInfo, 0, len(nodes))
	for _, node := range nodes {
		info, ok := readProcessInfo(procRoot, node.pid, node.depth, rootPID, ticksPerSecond)
		if !ok {
			continue
		}
		procs = append(procs, info)
	}
	return procs
}

func sampleProcessTreeFromProc(procRoot string, rootPID int, ticksPerSecond int64) ResourceUsage {
	if rootPID <= 0 || ticksPerSecond <= 0 {
		return ResourceUsage{}
	}

	nodes := walkProcessTree(procRoot, rootPID)
	var rssKB int64
	var cpuMs int64
	for _, node := range nodes {
		info, ok := readProcessInfo(procRoot, node.pid, node.depth, rootPID, ticksPerSecond)
		if !ok {
			continue
		}
		rssKB += info.RSSKB
		cpuMs += info.CPUTime
	}
	return ResourceUsage{PeakRSSKB: rssKB, CPUTimeMs: cpuMs}
}

func walkProcessTree(procRoot string, rootPID int) []processTreeNode {
	queue := []processTreeNode{{pid: rootPID, depth: 0}}
	seen := map[int]struct{}{}
	var ordered []processTreeNode

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node.pid <= 0 {
			continue
		}
		if _, ok := seen[node.pid]; ok {
			continue
		}
		seen[node.pid] = struct{}{}
		ordered = append(ordered, node)
		for _, child := range readChildren(procRoot, node.pid) {
			queue = append(queue, processTreeNode{pid: child, depth: node.depth + 1})
		}
	}
	return ordered
}

func readChildren(procRoot string, pid int) []int {
	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "task", strconv.Itoa(pid), "children"))
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(data))
	children := make([]int, 0, len(fields))
	for _, field := range fields {
		child, err := strconv.Atoi(field)
		if err != nil || child <= 0 {
			continue
		}
		children = append(children, child)
	}
	return children
}

func readProcessInfo(procRoot string, pid int, depth int, rootPID int, ticksPerSecond int64) (ProcessInfo, bool) {
	dir := filepath.Join(procRoot, strconv.Itoa(pid))
	statData, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return ProcessInfo{}, false
	}
	ps, err := parseProcStatFull(statData)
	if err != nil {
		return ProcessInfo{}, false
	}

	var rssKB int64
	if statusData, err := os.ReadFile(filepath.Join(dir, "status")); err == nil {
		rssKB = readStatusValueKB(statusData, "VmRSS:")
	}

	cmdline := ""
	if clData, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		cmdline = strings.ReplaceAll(string(clData), "\x00", " ")
		cmdline = strings.TrimSpace(cmdline)
	}

	cpuMs := int64(ps.utimeTicks+ps.stimeTicks) * 1000 / ticksPerSecond
	return ProcessInfo{
		PID:      ps.pid,
		PPID:     ps.ppid,
		PGID:     ps.pgrp,
		Depth:    depth,
		Role:     processRole(ps.pid, rootPID, depth, ps.comm, cmdline),
		State:    ps.state,
		IsActive: isActiveProcessState(ps.state),
		Comm:     ps.comm,
		RSSKB:    rssKB,
		CPUTime:  cpuMs,
		Cmdline:  cmdline,
	}, true
}

func markProcessActivity(before []ProcessInfo, after []ProcessInfo) []ProcessInfo {
	if len(after) == 0 {
		return nil
	}
	cpuByPID := make(map[int]int64, len(before))
	for _, p := range before {
		cpuByPID[p.PID] = p.CPUTime
	}
	for i := range after {
		if prevCPU, ok := cpuByPID[after[i].PID]; ok {
			delta := after[i].CPUTime - prevCPU
			if delta > 0 {
				after[i].CPUDeltaMs = delta
			}
		}
		after[i].IsActive = after[i].CPUDeltaMs > 0 || isActiveProcessState(after[i].State)
	}
	return after
}

func processRole(pid int, rootPID int, depth int, comm string, cmdline string) string {
	if pid == rootPID || depth == 0 {
		return "root"
	}
	text := strings.ToLower(comm + " " + cmdline)
	switch {
	case strings.Contains(text, "brun"):
		return "runner"
	case comm == "bash" || comm == "sh" || comm == "zsh" || strings.Contains(text, "/bin/bash") || strings.Contains(text, "/bin/sh"):
		return "shell"
	default:
		return "worker"
	}
}

func parseProcStat(data []byte) (procStat, error) {
	text := strings.TrimSpace(string(data))
	open := strings.IndexByte(text, '(')
	close := strings.LastIndexByte(text, ')')
	if open < 1 || close <= open {
		return procStat{}, errors.New("invalid proc stat comm")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(text[:open]))
	if err != nil {
		return procStat{}, err
	}

	fields := strings.Fields(strings.TrimSpace(text[close+1:]))
	if len(fields) <= 12 {
		return procStat{}, errors.New("invalid proc stat fields")
	}
	pgrp, err := strconv.Atoi(fields[2])
	if err != nil {
		return procStat{}, err
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return procStat{}, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return procStat{}, err
	}

	return procStat{
		pid:        pid,
		pgrp:       pgrp,
		utimeTicks: utime,
		stimeTicks: stime,
	}, nil
}

func parseProcStatFull(data []byte) (procStatFull, error) {
	text := strings.TrimSpace(string(data))
	open := strings.IndexByte(text, '(')
	close := strings.LastIndexByte(text, ')')
	if open < 1 || close <= open {
		return procStatFull{}, errors.New("invalid proc stat comm")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(text[:open]))
	if err != nil {
		return procStatFull{}, err
	}
	comm := text[open+1 : close]

	fields := strings.Fields(strings.TrimSpace(text[close+1:]))
	if len(fields) <= 12 {
		return procStatFull{}, errors.New("invalid proc stat fields")
	}
	state := fields[0]
	ppid, _ := strconv.Atoi(fields[1])
	pgrp, err := strconv.Atoi(fields[2])
	if err != nil {
		return procStatFull{}, err
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return procStatFull{}, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return procStatFull{}, err
	}

	return procStatFull{
		pid:        pid,
		ppid:       ppid,
		pgrp:       pgrp,
		state:      state,
		comm:       comm,
		utimeTicks: utime,
		stimeTicks: stime,
	}, nil
}

func readStatusValueKB(data []byte, key string) int64 {
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, key) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return value
	}
	return 0
}

func clockTicksPerSecond() int64 {
	return 100
}

func readProcStats(pgid int) (peakRSSKB, cpuTimeMs int64) {
	usage := sampleProcessGroup(pgid)
	return usage.PeakRSSKB, usage.CPUTimeMs
}

func killProcessGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}
