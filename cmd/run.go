package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type RunResult struct {
	ExitCode             int
	Status               string
	DurationMs           int64
	StartedAt            string
	EndedAt              string
	PeakRSSKB            int64
	CPUTimeMs            int64
	ResourceSupported    bool
	ResourceStatus       string
	TerminationReason    string
	TerminationSignal    string
	TerminationEscalated bool
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

// SaveInputScript 将首个参数（如果是文件）的源码快照保存到 run 目录
func SaveInputScript(runDir, scriptPath string) error {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return err
	}
	name := filepath.Base(scriptPath)
	return os.WriteFile(filepath.Join(runDir, "script."+name), data, 0644)
}

// ExecuteCommandWithSignal 执行命令并支持信号中断
func ExecuteCommandWithSignal(args []string, cwd, stdoutPath, stderrPath string, timeout time.Duration, sigCh chan os.Signal) RunResult {
	start := time.Now()

	stdoutF, _ := os.Create(stdoutPath)
	defer stdoutF.Close()
	stderrF, _ := os.Create(stderrPath)
	defer stderrF.Close()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = cwd
	cmd.Stdout = io.MultiWriter(os.Stdout, stdoutF)
	cmd.Stderr = io.MultiWriter(os.Stderr, stderrF)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err := cmd.Start()
	if err != nil {
		return RunResult{
			ExitCode:          1,
			Status:            "failed",
			DurationMs:        time.Since(start).Milliseconds(),
			StartedAt:         start.UTC().Format(time.RFC3339),
			EndedAt:           time.Now().UTC().Format(time.RFC3339),
			ResourceSupported: ResourceSupported(),
			ResourceStatus:    resourceStatus(ResourceUsage{}),
		}
	}
	runDir := filepath.Dir(stdoutPath)
	_ = os.Remove(filepath.Join(runDir, TerminationRecordFile))
	metadata, metadataErr := NewProcessMetadata(cmd.Process.Pid, cmd.Process.Pid)
	if metadataErr == nil {
		metadataErr = WriteProcessMetadata(runDir, metadata)
	}
	if metadataErr != nil {
		_ = KillProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		return RunResult{
			ExitCode:          1,
			Status:            "failed",
			DurationMs:        time.Since(start).Milliseconds(),
			StartedAt:         start.UTC().Format(time.RFC3339),
			EndedAt:           time.Now().UTC().Format(time.RFC3339),
			ResourceSupported: ResourceSupported(),
			ResourceStatus:    "unavailable",
		}
	}

	sampler := StartProcessGroupSampler(metadata.PGID, 500*time.Millisecond)

	// 等待命令完成或收到信号
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var timeoutChannel <-chan time.Time
	var timer *time.Timer
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		timeoutChannel = timer.C
		defer timer.Stop()
	}
	localReason := ""
	var controlResult StopResult
	select {
	case err = <-done:
	case <-sigCh:
		localReason = "signal"
		fmt.Printf("\n[信号] 收到中断信号，正在优雅停止...\n")
		controlResult = StopManagedProcess(runDir, metadata, 2, false, localReason)
		if !controlResult.OK {
			_ = KillProcessGroup(metadata.PGID, syscall.SIGKILL)
		}
		err = <-done
	case <-timeoutChannel:
		localReason = "timeout"
		controlResult = StopManagedProcess(runDir, metadata, 0, true, localReason)
		if !controlResult.OK {
			_ = KillProcessGroup(metadata.PGID, syscall.SIGKILL)
		}
		err = <-done
	}

	termination, _ := ReadTerminationRecord(runDir)
	if termination.Reason == "" {
		termination.Reason = localReason
	}
	if controlResult.Signal != "" {
		termination.Signal = controlResult.Signal
		termination.Escalated = controlResult.Escalated
	}
	exitCode := 0
	status := "success"
	switch termination.Reason {
	case "user", "signal":
		status = "cancelled"
		exitCode = 130
	case "timeout":
		status = "timed_out"
		exitCode = 124
	default:
		if err != nil {
			status = "failed"
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
	}
	if status == "cancelled" && exitCode == 0 {
		exitCode = 130
	}

	duration := time.Since(start)
	usage := sampler.Stop()

	return RunResult{
		ExitCode:             exitCode,
		Status:               status,
		DurationMs:           duration.Milliseconds(),
		StartedAt:            start.UTC().Format(time.RFC3339),
		EndedAt:              time.Now().UTC().Format(time.RFC3339),
		PeakRSSKB:            usage.PeakRSSKB,
		CPUTimeMs:            usage.CPUTimeMs,
		ResourceSupported:    ResourceSupported(),
		ResourceStatus:       resourceStatus(usage),
		TerminationReason:    termination.Reason,
		TerminationSignal:    termination.Signal,
		TerminationEscalated: termination.Escalated,
	}
}

func resourceStatus(usage ResourceUsage) string {
	if !ResourceSupported() {
		return "unsupported"
	}
	if usage.PeakRSSKB > 0 || usage.CPUTimeMs > 0 {
		return "ok"
	}
	return "unavailable"
}
