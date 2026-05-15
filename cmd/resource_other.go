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
