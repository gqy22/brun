package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRun_Success(t *testing.T) {
	tmp := t.TempDir()
	stdoutPath := filepath.Join(tmp, "stdout.log")
	stderrPath := filepath.Join(tmp, "stderr.log")

	result := ExecuteCommand([]string{"echo", "hello"}, tmp, stdoutPath, stderrPath, 0)

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Status != "success" {
		t.Errorf("Status = %q, want success", result.Status)
	}
	if result.DurationMs < 1 {
		t.Error("DurationMs should be > 0")
	}

	data, _ := os.ReadFile(stdoutPath)
	if !strings.Contains(string(data), "hello") {
		t.Errorf("stdout.log = %q, want contain hello", string(data))
	}
}

func TestRun_Failure(t *testing.T) {
	tmp := t.TempDir()
	result := ExecuteCommand([]string{"ls", "/nonexistent"}, tmp,
		filepath.Join(tmp, "out"), filepath.Join(tmp, "err"), 0)

	if result.ExitCode == 0 {
		t.Error("ExitCode should be non-zero for failed command")
	}
	if result.Status != "failed" {
		t.Errorf("Status = %q, want failed", result.Status)
	}
}

func TestRun_StderrCapture(t *testing.T) {
	tmp := t.TempDir()
	errPath := filepath.Join(tmp, "stderr.log")

	ExecuteCommand([]string{"sh", "-c", "echo error >&2"}, tmp,
		filepath.Join(tmp, "out"), errPath, 0)

	data, _ := os.ReadFile(errPath)
	if !strings.Contains(string(data), "error") {
		t.Errorf("stderr.log = %q, want contain error", string(data))
	}
}

func TestRun_Timeout(t *testing.T) {
	tmp := t.TempDir()
	start := time.Now()

	result := ExecuteCommand([]string{"sleep", "10"}, tmp,
		filepath.Join(tmp, "out"), filepath.Join(tmp, "err"), 100*time.Millisecond)

	if result.ExitCode == 0 {
		t.Error("timed out command should have non-zero exit code")
	}
	if result.Status != "failed" {
		t.Errorf("Status = %q, want failed", result.Status)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("timeout took too long")
	}
}

func TestRun_RealtimeOutput(t *testing.T) {
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	result := ExecuteCommandWithWriter(
		[]string{"sh", "-c", "echo outmsg; echo errmsg >&2"},
		&stdoutBuf, &stderrBuf, 0,
	)

	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", result.ExitCode)
	}
	if !strings.Contains(stdoutBuf.String(), "outmsg") {
		t.Errorf("stdout = %q, want outmsg", stdoutBuf.String())
	}
	if !strings.Contains(stderrBuf.String(), "errmsg") {
		t.Errorf("stderr = %q, want errmsg", stderrBuf.String())
	}
}

func TestBuildRunRecord(t *testing.T) {
	runID := "20260513-153012-a8f3c2"
	record := BuildRunRecord(runID, "test-project", "/work/dir",
		[]string{"python", "script.py"}, "/tmp/run-dir")

	if record.ID != runID {
		t.Errorf("ID = %q, want %q", record.ID, runID)
	}
	if record.Project != "test-project" {
		t.Errorf("Project = %q, want test-project", record.Project)
	}
	if record.Command != "python script.py" {
		t.Errorf("Command = %q, want python script.py", record.Command)
	}
	if record.Status != "running" {
		t.Errorf("Status = %q, want running", record.Status)
	}
	if record.StartedAt == "" {
		t.Error("StartedAt should be set")
	}
}
