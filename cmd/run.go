package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type RunResult struct {
	ExitCode   int
	Status     string
	DurationMs int64
	StartedAt  string
	EndedAt    string
}

type RunRecord struct {
	ID        string
	Name      string
	Project   string
	CWD       string
	Command   string
	Status    string
	RunDir    string
	StartedAt string
}

func ExecuteCommand(args []string, cwd, stdoutPath, stderrPath string, timeout time.Duration) RunResult {
	start := time.Now()

	stdoutF, _ := os.Create(stdoutPath)
	defer stdoutF.Close()
	stderrF, _ := os.Create(stderrPath)
	defer stderrF.Close()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = cwd
	cmd.Stdout = io.MultiWriter(os.Stdout, stdoutF)
	cmd.Stderr = io.MultiWriter(os.Stderr, stderrF)

	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() { cmd.Process.Kill() })
		defer timer.Stop()
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	duration := time.Since(start)
	status := "success"
	if exitCode != 0 {
		status = "failed"
	}

	return RunResult{
		ExitCode:   exitCode,
		Status:     status,
		DurationMs: duration.Milliseconds(),
		StartedAt:  start.UTC().Format(time.RFC3339),
		EndedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

func ExecuteCommandWithWriter(args []string, stdout, stderr io.Writer, timeout time.Duration) RunResult {
	start := time.Now()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() { cmd.Process.Kill() })
		defer timer.Stop()
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	duration := time.Since(start)
	status := "success"
	if exitCode != 0 {
		status = "failed"
	}

	return RunResult{
		ExitCode:   exitCode,
		Status:     status,
		DurationMs: duration.Milliseconds(),
		StartedAt:  start.UTC().Format(time.RFC3339),
		EndedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

func BuildRunRecord(runID, project, cwd string, args []string, runDir string) *RunRecord {
	now := time.Now().UTC().Format(time.RFC3339)
	return &RunRecord{
		ID:        runID,
		Project:   project,
		CWD:       cwd,
		Command:   strings.Join(args, " "),
		Status:    "running",
		RunDir:    runDir,
		StartedAt: now,
	}
}

func SaveCommandFile(runDir, command string) error {
	return os.WriteFile(filepath.Join(runDir, "command.sh"), []byte(command+"\n"), 0644)
}

func SaveEnvFile(runDir string) error {
	var buf bytes.Buffer
	envKeys := []string{"PATH", "HOME", "USER", "SHELL", "LANG", "CONDA_DEFAULT_ENV", "CONDA_PREFIX"}
	for _, k := range envKeys {
		if v := os.Getenv(k); v != "" {
			fmt.Fprintf(&buf, "%s=%s\n", k, v)
		}
	}
	return os.WriteFile(filepath.Join(runDir, "env.txt"), buf.Bytes(), 0644)
}
