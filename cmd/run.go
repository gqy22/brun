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

const second = time.Second

type RunResult struct {
	ExitCode   int
	Status     string
	DurationMs int64
	StartedAt  string
	EndedAt    string
	PeakRSSKB  int64
	CPUTimeMs  int64
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
			ExitCode:   1,
			Status:     "failed",
			DurationMs: time.Since(start).Milliseconds(),
			StartedAt:  start.UTC().Format(time.RFC3339),
			EndedAt:    time.Now().UTC().Format(time.RFC3339),
		}
	}
	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() {
			if cmd.Process != nil {
				killProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
			}
		})
		defer timer.Stop()
	}

	sampler := StartProcessGroupSampler(cmd.Process.Pid, 500*time.Millisecond)

	// 保存 PID 到 .pid 文件（供 Web kill 接口使用）
	pidFile := filepath.Join(filepath.Dir(stdoutPath), ".pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0644)

	// 等待命令完成或收到信号
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	interrupted := false
	select {
	case err = <-done:
		// 正常完成
	case <-sigCh:
		interrupted = true
		// 收到信号，优雅终止子进程组
		if cmd.Process.Pid > 0 {
			killProcessGroup(cmd.Process.Pid, syscall.SIGTERM)
			select {
			case waitErr := <-done:
				err = waitErr
			case <-time.After(2 * second):
				killProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
				err = <-done
			}
		}
		if err == nil {
			err = fmt.Errorf("被信号中断")
		}
	}

	exitCode := 0
	if err != nil {
		if interrupted {
			exitCode = 130
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 130 // SIGINT 的标准退出码
		}
	}

	duration := time.Since(start)
	status := "success"
	if exitCode != 0 {
		status = "failed"
	}
	usage := sampler.Stop()

	return RunResult{
		ExitCode:   exitCode,
		Status:     status,
		DurationMs: duration.Milliseconds(),
		StartedAt:  start.UTC().Format(time.RFC3339),
		EndedAt:    time.Now().UTC().Format(time.RFC3339),
		PeakRSSKB:  usage.PeakRSSKB,
		CPUTimeMs:  usage.CPUTimeMs,
	}
}
