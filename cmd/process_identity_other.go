//go:build !linux

package cmd

import "syscall"

func processStartTimeTicks(_ int) (uint64, error) { return 0, nil }

func inspectProcess(metadata ProcessMetadata) ProcessInspection {
	rootAlive := syscall.Kill(metadata.PID, 0) == nil
	return ProcessInspection{
		RootExists:    rootAlive,
		IdentityMatch: rootAlive,
		GroupAlive:    processGroupAlive(metadata.PGID),
		ActualPGID:    metadata.PGID,
	}
}

func processGroupAlive(pgid int) bool {
	return pgid > 0 && syscall.Kill(-pgid, 0) == nil
}
