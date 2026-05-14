package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() { cmd.Process.Kill() })
		defer timer.Stop()
	}

	err := cmd.Start()
	if err != nil {
		return RunResult{
			ExitCode:  1,
			Status:    "failed",
			DurationMs: time.Since(start).Milliseconds(),
			StartedAt: start.UTC().Format(time.RFC3339),
			EndedAt:   time.Now().UTC().Format(time.RFC3339),
		}
	}

	// 保存 PID 到 .pid 文件（供 Web kill 接口使用）
	pidFile := filepath.Join(filepath.Dir(stdoutPath), ".pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0644)

	// 等待命令完成或收到信号
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err = <-done:
		// 正常完成
	case <-sigCh:
		// 收到信号，优雅终止子进程组
		if cmd.Process.Pid > 0 {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) // 终止进程组
			time.Sleep(2 * second)                          // 等待优雅退出
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // 强制杀死
		}
		err = fmt.Errorf("被信号中断")
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
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
	// 采集资源数据
	pss, cst := readProcStats(cmd.Process.Pid)

	return RunResult{
		ExitCode:   exitCode,
		Status:     status,
		DurationMs: duration.Milliseconds(),
		StartedAt:  start.UTC().Format(time.RFC3339),
		EndedAt:    time.Now().UTC().Format(time.RFC3339),
	PeakRSSKB:  pss,
	CPUTimeMs:  cst,
	}
}

// readProcStats 从 /proc/{pid}/status 读取 VmHWM（峰值内存 KB）
// 从 /proc/{pid}/stat 读取 utime+stime（CPU 时钟 tick 转 ms）
// 进程已退出时返回零值
func readProcStats(pid int) (peakRSSKB, cpuTimeMs int64) {
	if pid <= 0 {
		return 0, 0
	}

	// 读取 VmHWM (Peak Resident Set Size, 单位 KB)
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	if data, err := os.ReadFile(statusPath); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "VmHWM:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					if v, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						peakRSSKB = v
					}
				}
				break
			}
		}
	}

	// 读取 utime + stime (clock ticks → ms)
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	if data, err := os.ReadFile(statPath); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 14 {
			var utime, stime uint64
			if u, err := strconv.ParseUint(fields[13], 10, 64); err == nil {
				utime = u
			}
			if s, err := strconv.ParseUint(fields[14], 10, 64); err == nil {
				stime = s
			}
			ticksPerSec := int64(100)
			cpuTimeMs = (int64(utime) + int64(stime)) * 1000 / ticksPerSec
		}
	}

	return peakRSSKB, cpuTimeMs
}
