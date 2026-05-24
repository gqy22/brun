//go:build !linux

package cmd

import (
	"syscall"
	"time"
)

type ResourceUsage struct {
	PeakRSSKB int64
	CPUTimeMs int64
}

type ProcessInfo struct {
	PID     int    `json:"pid"`
	PPID    int    `json:"ppid"`
	Comm    string `json:"comm"`
	State   string `json:"state"`
	RSSKB   int64  `json:"rss_kb"`
	CPUTime int64  `json:"cpu_time_ms"`
	Cmdline string `json:"cmdline"`
}

type ProcessGroupSampler struct{}

func StartProcessGroupSampler(_ int, _ time.Duration) *ProcessGroupSampler {
	return &ProcessGroupSampler{}
}

func (s *ProcessGroupSampler) Stop() ResourceUsage {
	return ResourceUsage{}
}

func readProcStats(_ int) (peakRSSKB, cpuTimeMs int64) {
	return 0, 0
}

func killProcessGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}

func ListProcessGroup(_ int) []ProcessInfo {
	return nil
}
