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

type procStat struct {
	pid        int
	pgrp       int
	utimeTicks uint64
	stimeTicks uint64
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
