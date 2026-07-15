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

func StopRun(pid int, graceSeconds int, force bool) StopResult {
	return StopManagedProcess("", ProcessMetadata{PID: pid, PGID: pid, Legacy: true}, graceSeconds, force, "user")
}

func StopManagedProcess(runDir string, metadata ProcessMetadata, graceSeconds int, force bool, reason string) StopResult {
	pid := metadata.PID
	if err := syscall.Kill(pid, 0); err != nil {
		if err == syscall.ESRCH {
			return StopResult{OK: true, PID: pid, PGID: metadata.PGID, Msg: fmt.Sprintf("进程 %d 已不存在", pid), AlreadyDead: true, GroupGone: true}
		}
		return StopResult{OK: false, PID: pid, PGID: metadata.PGID, Msg: fmt.Sprintf("无法访问进程 %d: %v", pid, err)}
	}

	ReadProcStats(pid)
	if err := WriteTerminationRecord(runDir, TerminationRecord{Reason: reason, Signal: "SIGTERM"}); err != nil {
		return StopResult{PID: pid, PGID: metadata.PGID, Msg: fmt.Sprintf("写入终止审计记录失败，未发送信号: %v", err)}
	}

	if err := KillProcessGroup(metadata.PGID, syscall.SIGTERM); err != nil {
		syscall.Kill(pid, syscall.SIGTERM)
	}

	if force {
		time.Sleep(100 * time.Millisecond)
		KillProcessGroup(metadata.PGID, syscall.SIGKILL)
		return StopResult{OK: true, PID: pid, PGID: metadata.PGID, Msg: "已强制终止 (SIGKILL)", Signal: "SIGKILL"}
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
			KillProcessGroup(metadata.PGID, syscall.SIGKILL)
			return StopResult{OK: true, PID: pid, PGID: metadata.PGID, Msg: "未在宽限期内退出，已强制终止 (SIGKILL)", Signal: "SIGKILL", Escalated: true}
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
