//go:build linux

package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecuteCommandWithSignalTerminatesChildProcessGroup(t *testing.T) {
	tmp := t.TempDir()
	childPIDPath := filepath.Join(tmp, "child.pid")
	sigCh := make(chan os.Signal, 1)

	go func() {
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			if _, err := os.Stat(childPIDPath); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		sigCh <- syscall.SIGTERM
	}()

	result := ExecuteCommandWithSignal(
		[]string{"sh", "-c", "sleep 30 & echo $! > child.pid; wait"},
		tmp,
		filepath.Join(tmp, "stdout.log"),
		filepath.Join(tmp, "stderr.log"),
		0,
		sigCh,
	)
	if result.ExitCode != 130 {
		t.Fatalf("ExitCode = %d, want 130", result.ExitCode)
	}

	data, err := os.ReadFile(childPIDPath)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse child pid: %v", err)
	}

	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if err := syscall.Kill(childPID, 0); err == syscall.ESRCH {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child process %d still exists after group termination", childPID)
}
