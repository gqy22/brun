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
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	ex := "  # 生成 01_align.sh\n  brun init align\n\n  # 同名再次生成 → 01_align2.sh\n  brun init align\n\n  # 不同名 → 新编号\n  brun init call          # → 02_call.sh"
	return &cobra.Command{
		Use:     "init <名称>",
		Short:   "在当前目录生成脚本模板",
		Long:    "生成带标准注释头部的脚本模板。自动检测 conda 环境、计算编号（同名递增后缀）。",
		Example: ex,
		Args:    cobra.MaximumNArgs(1),
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
}

// --- run (核心编排) ---

func runCmd() *cobra.Command {
	var name, project, note string
	var tags []string
	var noFsDiff bool
	var allowExit string
	var timeout int
	var cwdFlag string
	var foreground bool

	c := &cobra.Command{
		Use:   "run -- <command...>",
		Short: "执行命令并记录运行信息 (默认 nohup 后台运行)",
		Long:  "执行命令并自动记录运行日志、环境信息、Git 状态和输出文件变更。默认以 nohup 方式后台运行，关闭终端不会中断任务。",
		Example: `  # 基本用法 (默认 nohup 后台运行，关终端不会中断)
  brun run -- bwa mem -t 16 ref.fa reads_*.fq > aligned.sam
  # 等效于: nohup bwa mem ... > ~/.local/share/brun/runs/<id>/stdout.o 2> ~/.local/share/brun/runs/<id>/stderr.er &

  # 带项目名和标签
  brun run -p genome-align -t hg38,pep-align -- bwa mem ref.fa reads.fq > aligned.sam

  # Snakemake 流程 (前台运行，方便调试)
  brun run -f -- snakemake -j 8

  # FastQC 质控，指定名称和备注
  brun run -n "qc-report" --note "样本质量控制" -- fastqc *.fastq.gz

  # samtools 允许特定非零退出码（如空区域）
  brun run --allow-exit 1,2 -- samtools view -b input.bam "chr1:1-1000"

  # 在指定目录运行 R 脚本
  brun run --cwd /data/project -- Rscript analysis.R

  # 设置超时 (1 小时)
  brun run --timeout 3600 -- hisat2 -x genome_idx -1 r1.fq -r2.fq`,
		RunE: func(c *cobra.Command, args []string) error {
			if foreground {
				return executeRun(args, name, project, note, tags,
					noFsDiff, allowExit, time.Duration(timeout)*time.Second, cwdFlag)
			}
			return detachRun(c, args, name, project, note, tags,
				noFsDiff, allowExit, timeout, cwdFlag)
		},
	}
	c.Flags().StringVarP(&name, "name", "n", "", "run 名称")
	c.Flags().StringVarP(&project, "project", "p", "", "项目名")
	c.Flags().StringVar(&note, "note", "", "备注")
	c.Flags().StringArrayVarP(&tags, "tag", "t", []string{}, "标签 (支持逗号分隔: -t align,hg38)")
	c.Flags().BoolVar(&noFsDiff, "no-fs-diff", false, "禁用文件系统 diff")
	c.Flags().StringVar(&allowExit, "allow-exit", "", "允许的非零退出码 (逗号分隔，如: 1,2,127)")
	c.Flags().IntVar(&timeout, "timeout", 0, "超时(秒)")
	c.Flags().StringVar(&cwdFlag, "cwd", "", "运行目录")
	c.Flags().BoolVarP(&foreground, "foreground", "f", false, "前台运行 (默认 nohup 后台)")
	return c
}

func executeRun(args []string, name, project, note string, tags []string,
	noFsDiff bool, allowExit string, timeout time.Duration, cwdFlag string) error {

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
	if cwd == "" {
		cwd = detectCWD(args[0])
	}

	// 2. 生成 run_id + 创建 run_dir
	runID := internal.GenerateRunID()
	runDir := internal.RunDir(runID)
	if err := internal.EnsureDir(runDir); err != nil {
		return fmt.Errorf("创建 run 目录失败: %w", err)
	}

	// 3. 识别 project + 读 brun.yaml
	projName, projRoot := internal.DetectProject(cwd, internal.WithCLIProject(project))
	cfgPath := filepath.Join(projRoot, "brun.yaml")
	var cfg internal.Config
	if data, err := os.ReadFile(cfgPath); err == nil {
		cfg, _ = internal.ParseConfig(data)
	}
	if project != "" {
		projName = project
	}

	// 4. 收集 Git 信息
	gitInfo := internal.CollectGitInfo(cwd)

	// 5. 构建命令字符串
	commandStr := strings.Join(args, " ")

	// 5.5 预检查: 命令可执行文件是否存在
	if exePath, err := exec.LookPath(args[0]); err != nil {
		return fmt.Errorf("命令 '%s' 未找到: %w\n  提示: 使用 'brun run -- <command>' 确保命令在 PATH 中或使用完整路径", args[0], err)
	} else {
		args[0] = exePath // exec.LookPath 返回规范化路径
	}

	// 6. 保存 command.sh + env.txt + 输入脚本快照
	cmd.SaveCommandFile(runDir, commandStr)
	cmd.SaveEnvFile(runDir)
	// 尝试从参数中找到实际的脚本文件（跳过解释器如 bash/python）
	if scriptPath := findScriptArg(args); scriptPath != "" {
		cmd.SaveInputScript(runDir, scriptPath)
	}

	// 7. 打印启动信息
	fmt.Printf("Run started: %s\n", runID)
	fmt.Printf("Project: %s\n", projName)
	fmt.Printf("Logs: %s\n", runDir)
	internal.Log().Info("run_started", "run_id", runID, "project", projName, "command", commandStr, "cwd", cwd)

	// 8. 打开 DB，写入 running 记录
	store, err := openStore()
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer store.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	runRecord := &internal.Run{
		ID:        runID,
		Name:      name,
		Project:   projName,
		CWD:       cwd,
		Command:   commandStr,
		Status:    "running",
		RunDir:    runDir,
		StartedAt: now,
		Hostname:  hostname(),
		Username:  username(),
		GitRepo:   gitInfo.Repo,
		GitBranch: gitInfo.Branch,
		GitCommit: gitInfo.Commit,
		GitDirty:  gitInfo.Dirty,
	}
	if err := store.CreateRun(runRecord); err != nil {
		return fmt.Errorf("写入数据库失败: %w", err)
	}

	// 9. before 快照（如果不禁用 fs-diff）
	var beforeSnapshot map[string]internal.FileInfo
	if !noFsDiff {
		beforeSnapshot, _ = internal.SnapshotDir(cwd, cfg.Ignore)
	}

	// 10. 执行命令（带信号处理）
	stdoutPath := filepath.Join(runDir, "stdout.o")
	stderrPath := filepath.Join(runDir, "stderr.er")

	// 设置信号处理：Ctrl+C 时优雅终止子进程
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		<-sigCh
		fmt.Printf("\n[信号] 收到中断信号，正在优雅停止...\n")
	}()

	result := cmd.ExecuteCommandWithSignal(args, cwd, stdoutPath, stderrPath, timeout, sigCh)

	// 11. after 快照 + diff
	if !noFsDiff {
		afterSnapshot, _ := internal.SnapshotDir(cwd, cfg.Ignore)
		created, modified, deleted := internal.DiffSnapshots(beforeSnapshot, afterSnapshot)

		for _, f := range created {
			absPath := filepath.Join(cwd, f.Path)
			info, _ := os.Stat(absPath)
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			store.CreateArtifact(&internal.Artifact{
				RunID:   runID,
				Kind:    internal.ClassifyArtifact(f.Path),
				Status:  "created",
				Path:    f.Path,
				AbsPath: absPath,
				Size:    size,
			})
		}
		for _, f := range modified {
			absPath := filepath.Join(cwd, f.Path)
			info, _ := os.Stat(absPath)
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			store.CreateArtifact(&internal.Artifact{
				RunID:   runID,
				Kind:    internal.ClassifyArtifact(f.Path),
				Status:  "modified",
				Path:    f.Path,
				AbsPath: absPath,
				Size:    size,
			})
		}
		for _, f := range deleted {
			store.CreateArtifact(&internal.Artifact{
				RunID:  runID,
				Kind:   internal.ClassifyArtifact(f.Path),
				Status: "deleted",
				Path:   f.Path,
			})
		}
	}

	// 12. 处理 tags 和 note
	for _, t := range tags {
		store.AddTag(runID, t)
	}
	if note != "" {
		store.AddNote(runID, note)
	}

	// 13. 确定最终状态 (allow-exit 覆盖)
	status := result.Status
	if status == "failed" && len(allowedExits) > 0 && allowedExits[result.ExitCode] {
		status = "success"
	}

	// 14. 更新 DB status
	if err := store.UpdateRunStatus(runID, status, result.ExitCode,
		result.EndedAt, result.DurationMs); err != nil {
		return fmt.Errorf("更新状态失败: %w", err)
	}

	// 保存资源数据
	store.UpdateRunResources(runID, result.PeakRSSKB, result.CPUTimeMs)

	// 15. 写 metadata.yaml
	runRecord.Status = status
	runRecord.ExitCode = result.ExitCode
	runRecord.EndedAt = result.EndedAt
	runRecord.DurationMs = result.DurationMs
	metaYAML := cmd.BuildMetadataYAML(runRecord)
	os.WriteFile(filepath.Join(runDir, "metadata.yaml"), []byte(metaYAML), 0644)

	// 16. 打印摘要
	fmt.Printf("\nCommand finished %s in %s\n", status, cmd.DurationString(result.DurationMs))
	internal.Log().Info("run_finished", "run_id", runID, "status", status, "exit_code", result.ExitCode, "duration", cmd.DurationString(result.DurationMs))
	if status == "failed" {
		if errData, err := os.ReadFile(stderrPath); err == nil {
			if lines := strings.Split(strings.TrimRight(string(errData), "\r\n"), "\n"); len(lines) > 0 {
				lastN := lines
				if len(lastN) > 5 {
					lastN = lastN[len(lastN)-5:]
				}
				fmt.Printf("stderr (last %d lines):\n", len(lastN))
				for _, l := range lastN {
					fmt.Printf("  %s\n", l)
				}
			}
		}
		fmt.Printf("完整日志: brun logs %s --stderr\n", runID)
	}
	arts, _ := store.GetArtifacts(runID)
	if len(arts) > 0 {
		fmt.Printf("Outputs detected: %d\n", len(arts))
	}

	return nil
}

// detachRun 将命令以后台 nohup 方式执行，等效于 nohup cmd > out.o 2> out.er &
func detachRun(c *cobra.Command, args []string, name, project, note string, tags []string,
	noFsDiff bool, allowExit string, timeout int, cwdFlag string) error {

	// 构建子进程参数: brun run --foreground [原有参数] -- <command>
	childArgs := []string{"run", "--foreground"}

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
	childArgs = append(childArgs, "--")
	childArgs = append(childArgs, args...)

	// 获取当前可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行路径失败: %w", err)
	}

	// 生成 run ID 用于输出文件命名
	runID := internal.GenerateRunID()

	// 输出目录: ~/.local/share/brun/runs/<run_id>/
	runDir := internal.RunDir(runID)
	os.MkdirAll(runDir, 0755)

	stdoutPath := filepath.Join(runDir, "stdout.o")
	stderrPath := filepath.Join(runDir, "stderr.er")

	stdoutF, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("创建 stdout 文件失败: %w", err)
	}
	stderrF, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		stdoutF.Close()
		return fmt.Errorf("创建 stderr 文件失败: %w", err)
	}

	cmd := exec.Command(exePath, childArgs...)
	cmd.Stdout = stdoutF
	cmd.Stderr = stderrF
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		stdoutF.Close()
		stderrF.Close()
		return fmt.Errorf("启动后台进程失败: %w", err)
	}
	stdoutF.Close()
	stderrF.Close()

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
func hostname() string {
	h, _ := os.Hostname()
	return h
}

func username() string {
	return os.Getenv("USER")
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
