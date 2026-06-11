//go:build !linux

package cmd

import (
	"fmt"
	"syscall"
	"time"
)

type ResourceUsage struct {
	PeakRSSKB int64
	CPUTimeMs int64
}

func ResourceSupported() bool { return false }

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

type ProcessGroupSampler struct{}

func StartProcessGroupSampler(_ int, _ time.Duration) *ProcessGroupSampler {
	return &ProcessGroupSampler{}
}

func (s *ProcessGroupSampler) Stop() ResourceUsage {
	return ResourceUsage{}
}

func ReadProcStats(_ int) (peakRSSKB, cpuTimeMs int64) {
	return 0, 0
}

func KillProcessGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}

// StopResult 统一终止操作的结果
type StopResult struct {
	OK          bool   `json:"ok"`
	PID         int    `json:"pid,omitempty"`
	Msg         string `json:"msg,omitempty"`
	AlreadyDead bool   `json:"already_dead,omitempty"`
}

// StopRun 终止指定 run 的进程。Web apiKill 和 CLI stopCmd 共用此函数。
// graceSeconds: SIGTERM 后等待优雅退出的秒数，0 表示不发信号直接返回。
func StopRun(pid int, graceSeconds int, force bool) StopResult {
	if err := syscall.Kill(pid, 0); err != nil {
		if err == syscall.ESRCH {
			return StopResult{OK: true, PID: pid, Msg: fmt.Sprintf("进程 %d 已不存在", pid), AlreadyDead: true}
		}
		return StopResult{OK: false, PID: pid, Msg: fmt.Sprintf("无法访问进程 %d: %v", pid, err)}
	}

	ReadProcStats(pid)

	if err := KillProcessGroup(pid, syscall.SIGTERM); err != nil {
		syscall.Kill(pid, syscall.SIGTERM)
	}

	if force {
		time.Sleep(100 * time.Millisecond)
		KillProcessGroup(pid, syscall.SIGKILL)
		return StopResult{OK: true, PID: pid, Msg: "已强制终止 (SIGKILL)"}
	}

	if graceSeconds <= 0 {
		return StopResult{OK: true, PID: pid, Msg: "已发送终止信号"}
	}

	deadline := time.After(time.Duration(graceSeconds) * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			KillProcessGroup(pid, syscall.SIGKILL)
			return StopResult{OK: true, PID: pid, Msg: "未在宽限期内退出，已强制终止 (SIGKILL)"}
		case <-ticker.C:
			if syscall.Kill(pid, 0) != nil {
				return StopResult{OK: true, PID: pid, Msg: "已终止 (SIGTERM)"}
			}
		}
	}
}

func ListProcessGroup(_ int) []ProcessInfo {
	return nil
}

func ListProcessTree(_ int) []ProcessInfo {
	return nil
}

func ListProcessTreeWithActivity(_ int, _ time.Duration) []ProcessInfo {
	return nil
}

func SampleProcessTree(_ int) ResourceUsage {
	return ResourceUsage{}
}
