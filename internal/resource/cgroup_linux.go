//go:build linux

package resource

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/cgroups/v3/cgroup2"
	cgroupstats "github.com/containerd/cgroups/v3/cgroup2/stats"
)

type CgroupScope struct {
	manager        *cgroup2.Manager
	mountpoint     string
	relativePath   string
	fullPath       string
	delegatedRoot  string
	supervisorPath string
	ownsHierarchy  bool
	controllers    []string
}

func NewCgroupScope(env Environment, runID string) (*CgroupScope, error) {
	if !env.Unified || !env.Delegated || env.FullPath == "" {
		return nil, fmt.Errorf("cgroup environment is not delegated")
	}
	if !validRunID(runID) {
		return nil, fmt.Errorf("invalid run id for cgroup path: %q", runID)
	}
	initialControl, err := os.ReadFile(filepath.Join(env.FullPath, "cgroup.subtree_control"))
	if err != nil {
		return nil, fmt.Errorf("read delegated controllers: %w", err)
	}
	supervisorPath := filepath.Join(env.FullPath, "supervisor")
	createdSupervisor := false
	if err := os.Mkdir(supervisorPath, 0o755); err != nil {
		if !os.IsExist(err) {
			return nil, fmt.Errorf("create supervisor cgroup: %w", err)
		}
	} else {
		createdSupervisor = true
	}
	if err := writeCgroupValue(filepath.Join(supervisorPath, "cgroup.procs"), strconv.Itoa(os.Getpid())); err != nil {
		return nil, fmt.Errorf("move supervisor into cgroup: %w", err)
	}
	if err := waitCgroupFileEmpty(filepath.Join(env.FullPath, "cgroup.procs"), 3*time.Second); err != nil {
		return nil, fmt.Errorf("wait for delegated root to become empty: %w", err)
	}
	enabledControllers, err := enableControllers(env.FullPath, env.Controllers)
	if err != nil {
		return nil, fmt.Errorf("enable delegated controllers: %w", err)
	}

	relative := filepath.Join(env.CurrentPath, "payload-"+runID)
	if !strings.HasPrefix(relative, "/") {
		relative = "/" + relative
	}
	manager, err := cgroup2.NewManager(env.Mountpoint, relative, &cgroup2.Resources{})
	if err != nil {
		return nil, fmt.Errorf("create payload cgroup: %w", err)
	}
	full, err := safeCgroupPath(env.Mountpoint, relative)
	if err != nil {
		_ = manager.Delete()
		return nil, err
	}
	return &CgroupScope{
		manager: manager, mountpoint: env.Mountpoint, relativePath: relative, fullPath: full,
		delegatedRoot: env.FullPath, supervisorPath: supervisorPath,
		ownsHierarchy: createdSupervisor && len(strings.Fields(string(initialControl))) == 0,
		controllers:   enabledControllers,
	}, nil
}

func waitCgroupFileEmpty(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(data)) == "" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("cgroup still contains processes: %s", strings.Join(strings.Fields(string(data)), ","))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func LoadCgroupScope(relativePath string) (*CgroupScope, error) {
	full, err := safeCgroupPath("/sys/fs/cgroup", relativePath)
	if err != nil {
		return nil, err
	}
	manager, err := cgroup2.Load(relativePath, cgroup2.WithMountpoint("/sys/fs/cgroup"))
	if err != nil {
		return nil, err
	}
	return &CgroupScope{manager: manager, mountpoint: "/sys/fs/cgroup", relativePath: relativePath, fullPath: full}, nil
}

func (s *CgroupScope) Backend() string { return BackendCgroupV2 }
func (s *CgroupScope) Path() string    { return s.relativePath }

func (s *CgroupScope) Attach(pid int) error {
	if s == nil || s.manager == nil || pid <= 0 {
		return fmt.Errorf("invalid cgroup attachment pid=%d", pid)
	}
	return s.manager.AddProc(uint64(pid))
}

func (s *CgroupScope) Verify(pid int) error {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return err
	}
	path, err := parseUnifiedPath(data)
	if err != nil {
		return err
	}
	if path != s.relativePath {
		return fmt.Errorf("pid %d cgroup=%s, want %s", pid, path, s.relativePath)
	}
	return nil
}

func (s *CgroupScope) Stats() (Stats, error) {
	metrics, err := s.manager.Stat()
	if err != nil {
		return Stats{}, err
	}
	result := statsFromMetrics(metrics)
	if peak, err := readUintFile(filepath.Join(s.fullPath, "pids.peak")); err == nil {
		result.PIDsPeak = int64(peak)
	}
	return result, nil
}

func (s *CgroupScope) Populated() (bool, error) {
	data, err := os.ReadFile(filepath.Join(s.fullPath, "cgroup.events"))
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "populated" {
			return fields[1] == "1", nil
		}
	}
	return false, fmt.Errorf("cgroup.events has no populated field")
}

func (s *CgroupScope) Kill() error { return s.manager.Kill() }

func (s *CgroupScope) Close() error {
	if s == nil || s.manager == nil {
		return nil
	}
	if err := s.manager.Delete(); err != nil {
		return err
	}
	// Scopes loaded by the reconciler do not own the supervisor hierarchy.
	if s.delegatedRoot == "" || s.supervisorPath == "" || !s.ownsHierarchy {
		return nil
	}
	if err := disableControllers(s.delegatedRoot, s.controllers); err != nil {
		return fmt.Errorf("disable delegated controllers: %w", err)
	}
	if err := writeCgroupValue(filepath.Join(s.delegatedRoot, "cgroup.procs"), strconv.Itoa(os.Getpid())); err != nil {
		return fmt.Errorf("restore supervisor to delegated root: %w", err)
	}
	if err := os.Remove(s.supervisorPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove supervisor cgroup: %w", err)
	}
	return nil
}

type CgroupTermination struct {
	Empty     bool
	Signal    string
	Escalated bool
}

func TerminateCgroup(relativePath string, grace time.Duration, force bool) (CgroupTermination, error) {
	scope, err := LoadCgroupScope(relativePath)
	if err != nil {
		if os.IsNotExist(err) {
			return CgroupTermination{Empty: true}, nil
		}
		return CgroupTermination{}, err
	}
	populated, err := scope.Populated()
	if err != nil {
		return CgroupTermination{}, err
	}
	if !populated {
		return CgroupTermination{Empty: true}, nil
	}
	result := CgroupTermination{}
	if !force {
		procs, listErr := scope.manager.Procs(true)
		if listErr != nil {
			return result, listErr
		}
		for _, pid := range procs {
			if signalErr := syscall.Kill(int(pid), syscall.SIGTERM); signalErr != nil && signalErr != syscall.ESRCH {
				return result, signalErr
			}
		}
		result.Signal = "SIGTERM"
		if empty, waitErr := WaitEmpty(scope, grace); waitErr != nil {
			return result, waitErr
		} else if empty {
			result.Empty = true
			return result, nil
		}
	}
	if err := scope.Kill(); err != nil {
		return result, err
	}
	result.Signal = "SIGKILL"
	result.Escalated = !force
	result.Empty, err = WaitEmpty(scope, 3*time.Second)
	return result, err
}

func statsFromMetrics(metrics *cgroupstats.Metrics) Stats {
	var result Stats
	if metrics == nil {
		return result
	}
	if cpu := metrics.GetCPU(); cpu != nil {
		result.CPUTimeMs = int64(cpu.GetUsageUsec() / 1000)
		result.CPUUserMs = int64(cpu.GetUserUsec() / 1000)
		result.CPUSystemMs = int64(cpu.GetSystemUsec() / 1000)
	}
	if memory := metrics.GetMemory(); memory != nil {
		result.MemoryPeakByte = int64(memory.GetMaxUsage())
	}
	if ioStats := metrics.GetIo(); ioStats != nil {
		for _, entry := range ioStats.GetUsage() {
			result.IOReadBytes += int64(entry.GetRbytes())
			result.IOWriteBytes += int64(entry.GetWbytes())
		}
	}
	if events := metrics.GetMemoryEvents(); events != nil {
		result.OOMKillCount = int64(events.GetOomKill())
	}
	if pids := metrics.GetPids(); pids != nil {
		result.PIDsPeak = int64(pids.GetCurrent())
	}
	return result
}

func enableControllers(root string, available []string) ([]string, error) {
	wanted := make([]string, 0, 4)
	enabled := make([]string, 0, 4)
	for _, controller := range []string{"cpu", "memory", "io", "pids"} {
		if containsString(available, controller) {
			wanted = append(wanted, "+"+controller)
			enabled = append(enabled, controller)
		}
	}
	if !containsString(available, "cpu") || !containsString(available, "memory") {
		return nil, fmt.Errorf("cpu and memory controllers are required; available=%v", available)
	}
	if err := writeCgroupValue(filepath.Join(root, "cgroup.subtree_control"), strings.Join(wanted, " ")); err != nil {
		return nil, err
	}
	return enabled, nil
}

func disableControllers(root string, controllers []string) error {
	if len(controllers) == 0 {
		return nil
	}
	values := make([]string, 0, len(controllers))
	for _, controller := range controllers {
		values = append(values, "-"+controller)
	}
	return writeCgroupValue(filepath.Join(root, "cgroup.subtree_control"), strings.Join(values, " "))
}

func writeCgroupValue(path, value string) error {
	return os.WriteFile(path, []byte(value), 0o644)
}

func readUintFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validRunID(runID string) bool {
	if len(runID) < 8 || len(runID) > 80 {
		return false
	}
	for _, r := range runID {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
