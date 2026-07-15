//go:build linux

package cmd

import (
	"os"
	"os/exec"
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
		nil,
	)
	if result.ExitCode != 130 {
		t.Fatalf("ExitCode = %d, want 130", result.ExitCode)
	}
	if result.Status != "cancelled" || result.TerminationReason != "signal" {
		t.Fatalf("result = %+v, want cancelled by signal", result)
	}
	metadata, err := ReadProcessMetadata(tmp)
	if err != nil {
		t.Fatalf("read process metadata: %v", err)
	}
	if metadata.PID <= 0 || metadata.PGID <= 0 || metadata.StartTimeTicks == 0 || metadata.Schema != 1 {
		t.Fatalf("process metadata = %+v", metadata)
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

func TestExecuteCommandWithSignalReportsTimeout(t *testing.T) {
	tmp := t.TempDir()
	result := ExecuteCommandWithSignal(
		[]string{"sleep", "30"},
		tmp,
		filepath.Join(tmp, "stdout.log"),
		filepath.Join(tmp, "stderr.log"),
		100*time.Millisecond,
		make(chan os.Signal),
		nil,
	)
	if result.Status != "timed_out" || result.ExitCode != 124 || result.TerminationReason != "timeout" {
		t.Fatalf("result = %+v, want timed_out/124", result)
	}
	record, err := ReadTerminationRecord(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if record.Reason != "timeout" || record.Signal != "SIGKILL" {
		t.Fatalf("termination record = %+v", record)
	}
}

func TestStopManagedProcessVerifiesIdentityAndGroupExit(t *testing.T) {
	tmp := t.TempDir()
	command := exec.Command("sh", "-c", "sleep 30")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	metadata, err := NewProcessMetadata(command.Process.Pid, command.Process.Pid)
	if err != nil {
		_ = command.Process.Kill()
		t.Fatal(err)
	}
	bad := metadata
	bad.StartTimeTicks++
	refused := StopManagedProcess(tmp, bad, 1, false, "user")
	if refused.OK || !refused.IdentityMismatch {
		_ = command.Process.Kill()
		t.Fatalf("identity mismatch result = %+v", refused)
	}

	stopped := StopManagedProcess(tmp, metadata, 1, false, "user")
	_ = command.Wait()
	if !stopped.OK || !stopped.GroupGone {
		t.Fatalf("stop result = %+v", stopped)
	}
	if InspectProcess(metadata).GroupAlive {
		t.Fatal("process group is still alive")
	}
}
