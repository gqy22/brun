//go:build !linux

package cmd

import (
	"syscall"

	"github.com/shirou/gopsutil/v4/process"
)

func processStartTimeTicks(pid int) (uint64, error) {
	item, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0, err
	}
	created, err := item.CreateTime()
	return uint64(created), err
}

func inspectProcess(metadata ProcessMetadata) ProcessInspection {
	rootAlive := syscall.Kill(metadata.PID, 0) == nil
	start, _ := processStartTimeTicks(metadata.PID)
	return ProcessInspection{
		RootExists:    rootAlive,
		IdentityMatch: rootAlive && start == metadata.StartTimeTicks,
		GroupAlive:    processGroupAlive(metadata.PGID),
		ActualPGID:    metadata.PGID,
		ActualStart:   start,
	}
}

func processGroupAlive(pgid int) bool {
	return pgid > 0 && syscall.Kill(-pgid, 0) == nil
}
