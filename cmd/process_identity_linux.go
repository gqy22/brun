//go:build linux

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func processStartTimeTicks(pid int) (uint64, error) {
	stat, err := readProcessStatFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	if stat.startTimeTicks == 0 {
		return 0, errors.New("process start time unavailable")
	}
	return stat.startTimeTicks, nil
}

func inspectProcess(metadata ProcessMetadata) ProcessInspection {
	result := ProcessInspection{GroupAlive: processGroupAlive(metadata.PGID)}
	stat, err := readProcessStatFile(filepath.Join("/proc", strconv.Itoa(metadata.PID), "stat"))
	if err != nil {
		return result
	}
	result.RootExists = stat.state != "Z"
	result.ActualPGID = stat.pgrp
	result.ActualStart = stat.startTimeTicks
	result.IdentityMatch = result.RootExists && stat.pgrp == metadata.PGID &&
		(metadata.StartTimeTicks == 0 || stat.startTimeTicks == metadata.StartTimeTicks)
	return result
}

func processGroupAlive(pgid int) bool {
	if pgid <= 0 {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return syscall.Kill(-pgid, 0) == nil
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		stat, err := readProcessStatFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err == nil && stat.pgrp == pgid && stat.state != "Z" {
			return true
		}
	}
	return false
}

func readProcessStatFile(path string) (procStatFull, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return procStatFull{}, err
	}
	return parseProcStatFull(data)
}
