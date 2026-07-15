package resource

import (
	"fmt"
	"strings"
	"time"
)

type Mode string

const (
	ModeAuto   Mode = "auto"
	ModeProc   Mode = "proc"
	ModeCgroup Mode = "cgroup"
)

func ParseMode(value string) (Mode, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(value)))
	if mode == "" {
		mode = ModeAuto
	}
	switch mode {
	case ModeAuto, ModeProc, ModeCgroup:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown resource backend %q (want auto, proc, or cgroup)", value)
	}
}

type Stats struct {
	CPUTimeMs      int64
	CPUUserMs      int64
	CPUSystemMs    int64
	PeakRSSKB      int64
	MemoryPeakByte int64
	IOReadBytes    int64
	IOWriteBytes   int64
	OOMKillCount   int64
	PIDsPeak       int64
}

type Scope interface {
	Backend() string
	Path() string
	Attach(pid int) error
	Verify(pid int) error
	Stats() (Stats, error)
	Populated() (bool, error)
	Kill() error
	Close() error
}

type Backend interface {
	Name() string
	Prepare(runID string) (Scope, error)
}

func WaitEmpty(scope Scope, timeout time.Duration) (bool, error) {
	if scope == nil {
		return true, nil
	}
	deadline := time.Now().Add(timeout)
	for {
		populated, err := scope.Populated()
		if err != nil {
			return false, err
		}
		if !populated {
			return true, nil
		}
		if timeout <= 0 || time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
}
