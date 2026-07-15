package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/biotools/brun/cmd"
	"github.com/biotools/brun/internal"
	resourcepkg "github.com/biotools/brun/internal/resource"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	ht := MustParse("init")
	c := &cobra.Command{
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := "script"
			if len(args) > 0 {
				name = args[0]
			}

			scriptName, _ := cmd.NextScriptName(".", name)
			scriptPath := filepath.Join(".", scriptName)

			if _, err := os.Stat(scriptPath); err == nil {
				return fmt.Errorf("%s 已存在，如需重新生成请先删除", scriptName)
			}

			condaInfo := cmd.DetectCondaEnv()
			created := time.Now().Format("2006-01-02 15:04")

			content := cmd.GenerateScriptTemplate(name, name, condaInfo, created)
			if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
				return fmt.Errorf("生成脚本失败: %w", err)
			}

			fmt.Printf("✓ %s\n", scriptName)
			if condaInfo != "" {
				fmt.Printf("  环境: %s\n", condaInfo)
			}
			fmt.Printf("  用法: brun run -- bash %s\n", scriptName)
			return nil
		},
	}
	ht.Inject(c)
	return c
}
func runCmd() *cobra.Command {
	ht := MustParse("run")
	var name, project, note string
	var tags []string
	var noFsDiff bool
	var allowExit string
	var timeout int
	var cwdFlag string
	var foreground bool
	var runIDFlag string
	var resourceBackend string

	c := &cobra.Command{
		Args: cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			resourceMode, err := resourcepkg.ParseMode(resourceBackend)
			if err != nil {
				return cliError("invalid_resource_backend", err.Error(), "使用 auto、proc 或 cgroup", err)
			}
			if foreground {
				return executeRun(args, name, project, note, tags,
					noFsDiff, allowExit, time.Duration(timeout)*time.Second, cwdFlag, runIDFlag, string(resourceMode))
			}
			return detachRun(c, args, name, project, note, tags,
				noFsDiff, allowExit, timeout, cwdFlag, string(resourceMode))
		},
	}
	c.Flags().StringVarP(&name, "name", "n", "", "run 名称")
	c.Flags().StringVarP(&project, "project", "p", "", "项目名")
	c.Flags().StringVar(&note, "note", "", "备注")
	c.Flags().StringArrayVarP(&tags, "tag", "t", []string{}, "标签 (支持逗号分隔: -t align,hg38)")
	c.Flags().BoolVar(&noFsDiff, "no-fs-diff", false, "禁用文件系统 diff 和自动输出检测")
	c.Flags().StringVar(&allowExit, "allow-exit", "", "允许的非零退出码 (逗号分隔，如: 1,2,127)")
	c.Flags().IntVar(&timeout, "timeout", 0, "超时秒数；0 表示不限制")
	c.Flags().StringVar(&cwdFlag, "cwd", "", "运行目录；不传时从脚本路径或当前目录推断")
	c.Flags().BoolVarP(&foreground, "foreground", "f", false, "前台运行 (默认 nohup 后台)")
	c.Flags().StringVar(&runIDFlag, "run-id", "", "内部使用: 指定 run ID")
	c.Flags().StringVar(&resourceBackend, "resource-backend", "auto", "资源后端: auto、proc 或 cgroup")
	c.Flags().MarkHidden("run-id")
	ht.Inject(c)
	return c
}

func executeRun(args []string, name, project, note string, tags []string,
	noFsDiff bool, allowExit string, timeout time.Duration, cwdFlag string, runIDFlag string, resourceMode string) error {

	// 后台模式: 忽略 SIGHUP，关闭终端后继续运行 (等效 nohup)
	signal.Ignore(syscall.SIGHUP)

	// 支持逗号分隔: -t align,hg38 等价于 -t align -t hg38
	tags = splitComma(tags)
	allowedExits := parseAllowExit(allowExit)

	if len(args) == 0 {
		return fmt.Errorf("需要指定要执行的命令，使用: brun run -- <command>")
	}

	// 1. 确定工作目录
	cwd := cwdFlag
	cwdSource := "explicit"
	if cwd == "" {
		cwd = detectCWD(args[0])
		cwdSource = "inferred"
	}

	// 2. 生成 run_id + 创建 run_dir
	runID := runIDFlag
	if runID == "" {
		runID = internal.GenerateRunID()
	}
	runDir := internal.RunDir(runID)
	if err := internal.EnsureDir(runDir); err != nil {
		return cliError("run_dir_create_failed", "创建 run 目录失败: "+err.Error(), "检查 BRUN_HOME、磁盘空间和目录权限", err)
	}
	diagnostics := internal.NewDiagnosticWriter(runDir)
	if cwdSource == "inferred" {
		diagnostics.Info("cwd_inferred", "已推断运行目录", cwd)
	}
	mode, err := resourcepkg.ParseMode(resourceMode)
	if err != nil {
		return cliError("invalid_resource_backend", err.Error(), "使用 auto、proc 或 cgroup", err)
	}
	resourceDecision, err := resourcepkg.Decide(mode)
	if err != nil {
		return cliError("resource_backend_unavailable", err.Error(),
			"使用 systemd-run --user --scope --property=Delegate=yes 获取委派，或改用 --resource-backend proc", err)
	}
	if resourceDecision.Fallback != "" {
		diagnostics.Warning("resource_backend_fallback", "资源后端已降级",
			fmt.Sprintf("requested=%s backend=%s reason=%s", resourceDecision.Requested, resourceDecision.Backend, resourceDecision.Fallback))
	}

	// 3. 识别 project + 读 brun.yaml
	projName, projRoot, projectSource, err := internal.DetectProject(cwd, internal.WithCLIProject(project))
	if err != nil {
		return cliError("config_parse_failed", "项目配置错误: "+err.Error(), "修复 brun.yaml 后重试；不会使用默认配置继续运行", err)
	}
	if projectSource == "inferred" {
		diagnostics.Info("project_inferred", "已推断项目名", projName)
	}
	cfgPath := filepath.Join(projRoot, "brun.yaml")
	var cfg internal.Config
	if data, err := os.ReadFile(cfgPath); err == nil {
		if parsed, parseErr := internal.ParseConfig(data); parseErr == nil {
			cfg = parsed
		} else {
			return cliError("config_parse_failed", "brun.yaml 解析失败: "+parseErr.Error(), "修复 brun.yaml 后重试；不会使用默认配置继续运行", parseErr)
		}
	}
	if project != "" {
		projName = project
	}

	// 4. 收集 Git 信息
	gitInfo := internal.CollectGitInfo(cwd)
	condaInfo := cmd.DetectCondaInfo()

	// 5. 构建命令字符串
	commandStr := strings.Join(args, " ")

	// 6. 保存 command.sh + env.txt + 输入脚本快照
	if err := cmd.SaveCommandFile(runDir, commandStr); err != nil {
		diagnostics.Warning("command_file_write_failed", "command.sh 写入失败", err.Error())
		return cliError("command_file_write_failed", "command.sh 写入失败，命令未启动: "+err.Error(), "检查 run 目录权限和磁盘空间", err)
	}
	if err := cmd.SaveEnvFile(runDir); err != nil {
		diagnostics.Warning("env_file_write_failed", "env.txt 写入失败", err.Error())
		return cliError("env_file_write_failed", "env.txt 写入失败，命令未启动: "+err.Error(), "检查 run 目录权限和磁盘空间", err)
	}
	// 尝试从参数中找到实际的脚本文件（跳过解释器如 bash/python）
	if scriptPath := findScriptArg(args); scriptPath != "" {
		if err := cmd.SaveInputScript(runDir, scriptPath); err != nil {
			diagnostics.Warning("script_snapshot_failed", "脚本快照保存失败", err.Error())
		}
	} else {
		diagnostics.Info("script_snapshot_missing", "未保存脚本快照", "未在命令参数中找到可读脚本文件")
	}

	// 7. 打印启动信息
	fmt.Printf("运行已启动: %s\n", runID)
	fmt.Printf("项目: %s\n", projName)
	fmt.Printf("日志: %s\n", runDir)
	internal.Log().Info("run_started", "run_id", runID, "project", projName, "command", commandStr, "cwd", cwd)

	// 8. 打开 DB，写入 running 记录
	store, err := openStore()
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer store.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	host, hostStatus := hostname()
	user, userStatus := username()
	runRecord := &internal.Run{
		ID:                runID,
		Name:              name,
		Project:           projName,
		CWD:               cwd,
		Command:           commandStr,
		Status:            "starting",
		RunDir:            runDir,
		StartedAt:         now,
		Hostname:          host,
		HostnameStatus:    hostStatus,
		Username:          user,
		UsernameStatus:    userStatus,
		GitRepo:           gitInfo.Repo,
		GitBranch:         gitInfo.Branch,
		GitCommit:         gitInfo.Commit,
		GitDirty:          gitInfo.Dirty,
		CondaStatus:       condaInfo.Status,
		CondaEnv:          condaInfo.Env,
		CondaPrefix:       condaInfo.Prefix,
		PythonVersion:     condaInfo.PythonVersion,
		CWDSource:         cwdSource,
		ProjectSource:     projectSource,
		ResourceRequested: resourceDecision.Requested,
		ResourceBackend:   resourceDecision.Backend,
		ResourceFallback:  resourceDecision.Fallback,
		ResourceSupported: resourceDecision.Backend != resourcepkg.BackendUnsupported,
	}
	if resourceDecision.Backend == resourcepkg.BackendCgroupV2 {
		runRecord.ResourceCgroupPath = resourceDecision.Env.CurrentPath
	}
	if err := store.CreateRun(runRecord); err != nil {
		return fmt.Errorf("写入数据库失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "metadata.yaml"), []byte(cmd.BuildMetadataYAML(runRecord)), 0o644); err != nil {
		diagnostics.Warning("metadata_write_failed", "starting metadata.yaml 写入失败", err.Error())
	}
	syncRunDiagnostics(store, runID, runDir)
	// 立即累加模式：每次写完 diagnostics.jsonl 并 fsync 后，对 SQLite 计数 +1，
	// 避免 SIGKILL/OOM kill 在 finishRun 之前发生时 display_status 漏报 warning。
	diagnostics.SetCounter(func(level, code, lastAt string) {
		if err := store.IncrementRunDiagnostic(runID, level, code, lastAt); err != nil {
			internal.Log().Warn("diagnostic_counter_increment_failed", "run_id", runID, "level", level, "error", err.Error())
		}
	})

	// 8.5 预检查: 命令可执行文件是否存在。此时 run 记录已创建，启动失败也可查询。
	if exePath, err := exec.LookPath(args[0]); err != nil {
		message := fmt.Sprintf("命令 %q 未找到: %v", args[0], err)
		diagnostics.Warning("command_not_found", message, "命令未启动")
		stderrPath := filepath.Join(runDir, "stderr.er")
		_ = os.WriteFile(stderrPath, []byte(message+"\n"), 0644)
		result := cmd.RunResult{
			ExitCode:          127,
			Status:            "failed",
			DurationMs:        0,
			StartedAt:         now,
			EndedAt:           time.Now().UTC().Format(time.RFC3339),
			ResourceSupported: cmd.ResourceSupported(),
			ResourceStatus:    "unavailable",
		}
		if err := finishRun(store, diagnostics, runRecord, result, "failed"); err != nil {
			return err
		}
		fmt.Printf("\n命令执行失败，耗时 %s\n", cmd.DurationString(result.DurationMs))
		fmt.Printf("stderr（最后 1 行）:\n  %s\n", message)
		fmt.Printf("完整日志: brun logs %s --stderr\n", runID)
		printDiagnosticSummary(runDir)
		return nil
	} else {
		args[0] = exePath // exec.LookPath 返回规范化路径
	}

	// 9. before 快照（如果不禁用 fs-diff）
	var beforeSnapshot map[string]internal.FileInfo
	fsDiffReady := false
	if !noFsDiff {
		if snapshot, err := internal.SnapshotDir(cwd, cfg.Ignore); err == nil {
			beforeSnapshot = snapshot
			fsDiffReady = true
		} else {
			diagnostics.Warning("fs_snapshot_before_failed", "运行前文件快照失败", err.Error())
		}
	}

	// 10. 执行命令（带信号处理）
	stdoutPath := filepath.Join(runDir, "stdout.o")
	stderrPath := filepath.Join(runDir, "stderr.er")

	// 设置信号处理：Ctrl+C 时优雅终止子进程
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	result := cmd.ExecuteCommandWithSignal(args, cwd, stdoutPath, stderrPath, timeout, sigCh, func(metadata cmd.ProcessMetadata) error {
		if err := store.MarkRunRunning(runID, metadata.PID, metadata.PGID, int64(metadata.StartTimeTicks)); err != nil {
			return err
		}
		runRecord.Status = "running"
		runRecord.ProcessPID = metadata.PID
		runRecord.ProcessPGID = metadata.PGID
		runRecord.ProcessStartTicks = int64(metadata.StartTimeTicks)
		return os.WriteFile(filepath.Join(runDir, "metadata.yaml"), []byte(cmd.BuildMetadataYAML(runRecord)), 0o644)
	})
	if result.TerminationReason != "" {
		detail := fmt.Sprintf("reason=%s signal=%s escalated=%t", result.TerminationReason, result.TerminationSignal, result.TerminationEscalated)
		switch result.TerminationReason {
		case "timeout":
			diagnostics.Warning("run_timed_out", "任务达到超时限制", detail)
		case "signal":
			diagnostics.Info("run_cancelled", "任务收到终止信号", detail)
		}
	}

	// 11. after 快照 + diff
	if !noFsDiff {
		afterSnapshot, err := internal.SnapshotDir(cwd, cfg.Ignore)
		if err != nil {
			diagnostics.Warning("fs_snapshot_after_failed", "运行后文件快照失败", err.Error())
			fsDiffReady = false
		}

		if fsDiffReady {
			created, modified, deleted := internal.DiffSnapshots(beforeSnapshot, afterSnapshot)

			for _, f := range created {
				absPath := filepath.Join(cwd, f.Path)
				info, _ := os.Stat(absPath)
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				if err := store.CreateArtifact(&internal.Artifact{
					RunID:   runID,
					Kind:    internal.ClassifyArtifact(f.Path),
					Status:  "created",
					Path:    f.Path,
					AbsPath: absPath,
					Size:    size,
				}); err != nil {
					diagnostics.Warning("artifact_write_failed", "输出文件记录写入失败", err.Error())
				}
			}
			for _, f := range modified {
				absPath := filepath.Join(cwd, f.Path)
				info, _ := os.Stat(absPath)
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				if err := store.CreateArtifact(&internal.Artifact{
					RunID:   runID,
					Kind:    internal.ClassifyArtifact(f.Path),
					Status:  "modified",
					Path:    f.Path,
					AbsPath: absPath,
					Size:    size,
				}); err != nil {
					diagnostics.Warning("artifact_write_failed", "输出文件记录写入失败", err.Error())
				}
			}
			for _, f := range deleted {
				if err := store.CreateArtifact(&internal.Artifact{
					RunID:  runID,
					Kind:   internal.ClassifyArtifact(f.Path),
					Status: "deleted",
					Path:   f.Path,
				}); err != nil {
					diagnostics.Warning("artifact_write_failed", "输出文件记录写入失败", err.Error())
				}
			}
		}
	}

	// 12. 处理 tags 和 note
	for _, t := range tags {
		if err := store.AddTag(runID, t); err != nil {
			diagnostics.Warning("tag_write_failed", "标签写入失败", err.Error())
		}
	}
	if note != "" {
		if err := store.AddNote(runID, note); err != nil {
			diagnostics.Warning("note_write_failed", "备注写入失败", err.Error())
		}
	}

	// 13. 确定最终状态 (allow-exit 覆盖)
	status := result.Status
	if status == "failed" && len(allowedExits) > 0 && allowedExits[result.ExitCode] {
		status = "success"
	}

	// 14-15. 更新 DB status、资源数据和 metadata.yaml
	if err := finishRun(store, diagnostics, runRecord, result, status); err != nil {
		return err
	}
	status = runRecord.Status

	// 16. 打印摘要
	fmt.Printf("\n命令执行完成: %s，耗时 %s\n", status, cmd.DurationString(result.DurationMs))
	internal.Log().Info("run_finished", "run_id", runID, "status", status, "exit_code", result.ExitCode, "duration", cmd.DurationString(result.DurationMs))
	if status == "failed" || status == "timed_out" {
		if errData, err := os.ReadFile(stderrPath); err == nil {
			if lines := strings.Split(strings.TrimRight(string(errData), "\r\n"), "\n"); len(lines) > 0 {
				lastN := lines
				if len(lastN) > 5 {
					lastN = lastN[len(lastN)-5:]
				}
				fmt.Printf("stderr（最后 %d 行）:\n", len(lastN))
				for _, l := range lastN {
					fmt.Printf("  %s\n", l)
				}
			}
		}
		fmt.Printf("完整日志: brun logs %s --stderr\n", runID)
	}
	arts, _ := store.GetArtifacts(runID)
	if len(arts) > 0 {
		fmt.Printf("检测到输出文件: %d\n", len(arts))
	}
	printDiagnosticSummary(runDir)

	return nil
}

func finishRun(store *internal.Store, diagnostics *internal.DiagnosticWriter, runRecord *internal.Run, result cmd.RunResult, status string) error {
	changed, err := store.FinalizeRun(runRecord.ID, status, result.ExitCode,
		result.EndedAt, result.DurationMs, result.TerminationReason, result.TerminationSignal, result.TerminationEscalated)
	if err != nil {
		return fmt.Errorf("更新状态失败: %w", err)
	}
	if err := store.UpdateRunResources(runRecord.ID, result.PeakRSSKB, result.CPUTimeMs, result.ResourceSupported, result.ResourceStatus); err != nil {
		diagnostics.Warning("resource_write_failed", "资源数据写入失败", err.Error())
	}

	if changed {
		runRecord.Status = status
		runRecord.ExitCode = result.ExitCode
		runRecord.EndedAt = result.EndedAt
		runRecord.DurationMs = result.DurationMs
		runRecord.TerminationReason = result.TerminationReason
		runRecord.TerminationSignal = result.TerminationSignal
		runRecord.TerminationEscalated = result.TerminationEscalated
	} else if latest, getErr := store.GetRun(runRecord.ID); getErr == nil {
		*runRecord = *latest
	}
	runRecord.PeakRSSKB = result.PeakRSSKB
	runRecord.CPUTimeMs = result.CPUTimeMs
	runRecord.ResourceSupported = result.ResourceSupported
	runRecord.ResourceStatus = result.ResourceStatus
	if summary, err := internal.ReadDiagnosticSummary(runRecord.RunDir); err == nil {
		runRecord.DiagInfoCount = summary.InfoCount
		runRecord.DiagWarningCount = summary.WarningCount
		runRecord.DiagErrorCount = summary.ErrorCount
		runRecord.DiagLastCode = summary.LastCode
		runRecord.DiagLastAt = summary.LastAt
		// 终态一致性：以 jsonl 真实计数为准再写一次 SQLite，
		// 覆盖掉期间任何 IncrementRunDiagnostic 漏掉的事件。
		_ = store.UpdateRunDiagnostics(runRecord.ID, summary)
	}
	if latest, getErr := store.GetRun(runRecord.ID); getErr == nil {
		*runRecord = *latest
	}
	metaYAML := cmd.BuildMetadataYAML(runRecord)
	if err := os.WriteFile(filepath.Join(runRecord.RunDir, "metadata.yaml"), []byte(metaYAML), 0644); err != nil {
		diagnostics.Warning("metadata_write_failed", "metadata.yaml 写入失败", err.Error())
	}
	return nil
}

func printDiagnosticSummary(runDir string) {
	events, err := internal.ReadDiagnostics(runDir)
	if err != nil {
		internal.Log().Warn("diagnostic_read_failed", "run_dir", runDir, "error", err.Error())
		return
	}
	warnings := internal.DiagnosticWarnings(events)
	if len(warnings) == 0 {
		return
	}
	fmt.Printf("诊断提示:\n")
	for _, warning := range warnings {
		if warning.Detail != "" {
			fmt.Printf("  ! %s：%s\n", warning.Message, warning.Detail)
		} else {
			fmt.Printf("  ! %s\n", warning.Message)
		}
	}
	fmt.Printf("诊断文件: %s\n", filepath.Join(runDir, internal.DiagnosticsFileName))
}

func syncRunDiagnostics(store *internal.Store, runID, runDir string) {
	if store == nil || runID == "" || runDir == "" {
		return
	}
	summary, err := internal.ReadDiagnosticSummary(runDir)
	if err != nil {
		internal.Log().Warn("diagnostic_summary_read_failed", "run_id", runID, "error", err.Error())
		return
	}
	if err := store.UpdateRunDiagnostics(runID, summary); err != nil {
		internal.Log().Warn("diagnostic_summary_update_failed", "run_id", runID, "error", err.Error())
	}
}

// detachRun 将命令以后台 nohup 方式执行，等效于 nohup cmd > out.o 2> out.er &
func detachRun(c *cobra.Command, args []string, name, project, note string, tags []string,
	noFsDiff bool, allowExit string, timeout int, cwdFlag string, resourceMode string) error {

	// 生成 run ID，父进程和子进程共享，确保提示、日志路径和数据库记录一致。
	runID := internal.GenerateRunID()

	// 构建子进程参数: brun run --foreground --run-id <id> [原有参数] -- <command>
	childArgs := []string{"run", "--foreground", "--run-id", runID}

	if name != "" {
		childArgs = append(childArgs, "--name", name)
	}
	if project != "" {
		childArgs = append(childArgs, "--project", project)
	}
	if note != "" {
		childArgs = append(childArgs, "--note", note)
	}
	for _, t := range tags {
		childArgs = append(childArgs, "--tag", t)
	}
	if noFsDiff {
		childArgs = append(childArgs, "--no-fs-diff")
	}
	if allowExit != "" {
		childArgs = append(childArgs, "--allow-exit", allowExit)
	}
	if timeout > 0 {
		childArgs = append(childArgs, "--timeout", strconv.Itoa(timeout))
	}
	if cwdFlag != "" {
		childArgs = append(childArgs, "--cwd", cwdFlag)
	}
	childArgs = append(childArgs, "--resource-backend", resourceMode)
	childArgs = append(childArgs, "--")
	childArgs = append(childArgs, args...)

	// 获取当前可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行路径失败: %w", err)
	}

	// 输出目录: ~/.brun/runs/YYYY/MM/DD/<run_id>/
	runDir := internal.RunDir(runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("创建 run 目录失败: %w", err)
	}

	stdoutPath := filepath.Join(runDir, "stdout.o")
	stderrPath := filepath.Join(runDir, "stderr.er")

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开空输出失败: %w", err)
	}

	cmd := exec.Command(exePath, childArgs...)
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		devNull.Close()
		return fmt.Errorf("启动后台进程失败: %w", err)
	}
	devNull.Close()

	fmt.Printf("[nohup] PID=%d, RunID=%s\n", cmd.Process.Pid, runID)
	fmt.Printf("[nohup] stdout: %s\n", stdoutPath)
	fmt.Printf("[nohup] stderr: %s\n", stderrPath)
	fmt.Printf("[nohup] 使用 'brun list' 查看运行状态\n")
	internal.Log().Info("run_detached", "run_id", runID, "pid", cmd.Process.Pid, "command", strings.Join(args, " "))
	return nil
}

// --- list ---

func splitComma(items []string) []string {
	var out []string
	for _, item := range items {
		for _, s := range strings.Split(item, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func parseAllowExit(s string) map[int]bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	result := make(map[int]bool)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			result[n] = true
		}
	}
	return result
}

// parseTimeFilter 将用户输入的时间过滤值转为 RFC3339
// 支持: "2026-05-13", "1h", "2d", "today", "1w"
func hostname() (string, string) {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h, "ok"
	}
	return "", "unavailable"
}

func username() (string, string) {
	if u := os.Getenv("USER"); u != "" {
		return u, "ok"
	}
	return "", "unavailable"
}

// detectCWD 智能检测工作目录：
//   - 如果首个参数是已存在的脚本文件（.sh/.py/.R/...），自动使用其所在目录
//   - 否则回退到当前目录
func detectCWD(firstArg string) string {
	// 脚本文件扩展名白名单
	scriptExts := map[string]bool{
		".sh": true, ".bash": true, ".zsh": true,
		".py": true, ".rb": true, ".pl": true,
		".R": true, ".r": true, ".Rmd": true,
		".js": true, ".ts": true,
		".nf": true, ".smk": true,
	}

	ext := filepath.Ext(firstArg)
	if !scriptExts[ext] {
		cwd, _ := os.Getwd()
		return cwd
	}

	absPath, err := filepath.Abs(firstArg)
	if err != nil {
		cwd, _ := os.Getwd()
		return cwd
	}

	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		cwd, _ := os.Getwd()
		return cwd
	}

	return filepath.Dir(absPath)
}

// findScriptArg 从命令参数中查找实际的脚本文件
// 跳过解释器（args[0] 如 bash/python），在后续参数中寻找文本脚本文件
func findScriptArg(args []string) string {
	if len(args) < 2 {
		return ""
	}

	// 脚本文件扩展名白名单
	scriptExts := map[string]bool{
		".sh": true, ".bash": true, ".zsh": true,
		".py": true, ".rb": true, ".pl": true,
		".R": true, ".r": true, ".Rmd": true,
		".js": true, ".ts": true,
		".nf": true, ".smk": true,
		".yaml": true, ".yml": true, ".json": true,
		".toml": true, ".cfg": true, ".conf": true,
	}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		// 跳过选项参数（以 - 开头）
		if strings.HasPrefix(arg, "-") {
			continue
		}
		info, err := os.Stat(arg)
		if err != nil || info.IsDir() {
			continue
		}
		ext := filepath.Ext(arg)
		if !scriptExts[ext] {
			continue
		}
		// 验证是文本文件（非二进制）：检查前 512 字节不含 NULL 字节
		if isTextFile(arg) {
			return arg
		}
	}
	return ""
}

// isTextFile 检查文件是否为文本文件（前 512 字节无 NULL 字节）
func isTextFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return n > 0 && !bytes.Contains(buf[:n], []byte{0})
}
