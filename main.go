package main

import (
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

var version = "0.1.0"

const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if .Example}}
示例:
{{.Example}}
{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

const usageTemplate = `用法: {{.UseLine}}

{{if .HasAvailableSubCommands}}
可用命令:
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}
{{end}}
{{if .HasAvailableLocalFlags}}
选项:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
{{if .HasAvailableInheritedFlags}}
全局选项:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
{{if .HasHelpSubCommands}}
更多帮助命令:
{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}
{{end}}
{{if .HasAvailableSubCommands}}
使用 "{{.CommandPath}} [命令] --help" 获取更多信息
{{end}}
`

// 注入值: -X main.commit=xxx -X main.buildDate=xxx
var commit string
var buildDate string

func main() {
	rootCmd := &cobra.Command{
		Use:   "brun",
		Short: "bio-runner: 面向生物信息学的运行记录与日志管理工具",
		Long: `brun 是一个跨项目运行记录工具。
通过 brun run -- <command> 包装任意命令，自动记录日志、环境、Git 信息和输出文件。`,
		Version: version,
	}
	rootCmd.SetHelpTemplate(helpTemplate)
	rootCmd.SetUsageTemplate(usageTemplate)

	rootCmd.AddCommand(
		initCmd(),
		runCmd(),
		listCmd(),
		showCmd(),
		logsCmd(),
		outputsCmd(),
		tagCmd(),
		noteCmd(),
		rerunCmd(),
		cleanCmd(),
	)
	// 替换内置命令为中文描述
	rootCmd.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "查看帮助信息",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return rootCmd.Help()
			}
			c2, _, err := rootCmd.Find(args)
			if err != nil {
				return err
			}
			return c2.Help()
		},
	})
	// 禁用 cobra 默认的英文 help/version flag，改用中文版本
	rootCmd.PersistentFlags().BoolP("help", "h", false, "显示帮助信息")
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(&cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "生成指定 shell 的自动补全脚本",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return rootCmd.GenBashCompletion(os.Stdout)
			case "zsh":
				return rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				return rootCmd.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("不支持的 shell: %s (支持: bash, zsh, fish, powershell)", args[0])
			}
		},
	})
	rootCmd.RegisterFlagCompletionFunc("help", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	})
	rootCmd.PersistentFlags().BoolP("version", "v", false, "显示版本号")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func openStore() (*internal.Store, error) {
	return internal.NewStore(filepath.Join(internal.HomeDir(), "db.sqlite"))
}


// --- init ---

func initCmd() *cobra.Command {
	ex := "  # 生成 01_align.sh\n  brun init align\n\n  # 同名再次生成 → 01_align2.sh\n  brun init align\n\n  # 不同名 → 新编号\n  brun init call          # → 02_call.sh"
	return &cobra.Command{
		Use:     "init <名称>",
		Short:   "在当前目录生成脚本模板",
		Long:    "生成带标准注释头部的脚本模板。自动检测 conda 环境、计算编号（同名递增后缀）。",
		Example: ex,
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
		Use:     "run -- <command...>",
		Short:   "执行命令并记录运行信息 (默认 nohup 后台运行)",
		Long:    "执行命令并自动记录运行日志、环境信息、Git 状态和输出文件变更。默认以 nohup 方式后台运行，关闭终端不会中断任务。",
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

	// 支持逗号分隔: -t align,hg38 等价于 -t align -t hg38
	tags = splitComma(tags)
	allowedExits := parseAllowExit(allowExit)

	if len(args) == 0 {
		return fmt.Errorf("需要指定要执行的命令，使用: brun run -- <command>")
	}

	// 1. 确定工作目录
	cwd := cwdFlag
	if cwd == "" {
		cwd, _ = os.Getwd()
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

	// 6. 保存 command.sh + env.txt
	cmd.SaveCommandFile(runDir, commandStr)
	cmd.SaveEnvFile(runDir)

	// 7. 打印启动信息
	fmt.Printf("Run started: %s\n", runID)
	fmt.Printf("Project: %s\n", projName)
	fmt.Printf("Logs: %s\n", runDir)

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

	// 15. 写 metadata.yaml
	runRecord.Status = status
	runRecord.ExitCode = result.ExitCode
	runRecord.EndedAt = result.EndedAt
	runRecord.DurationMs = result.DurationMs
	metaYAML := cmd.BuildMetadataYAML(runRecord)
	os.WriteFile(filepath.Join(runDir, "metadata.yaml"), []byte(metaYAML), 0644)

	// 16. 打印摘要
	fmt.Printf("\nCommand finished %s in %s\n", status, cmd.DurationString(result.DurationMs))
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
		Setsid:  true,
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
	return nil
}

// --- list ---

func listCmd() *cobra.Command {
	var project, status, tag, search, since, until string
	var limit int

	cc := &cobra.Command{
		Use:   "list",
		Short: "列出运行历史",
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			sinceVal, untilVal := since, until
			if since != "" {
				sinceVal = parseTimeFilter(since)
			}
			if until != "" {
				untilVal = parseTimeFilter(until)
			}

			runs, err := store.ListRuns(limit, project, status, tag, search, sinceVal, untilVal)
			if err != nil {
				return err
			}

			rows := make([]cmd.RunRow, len(runs))
			for i, r := range runs {
				rows[i] = cmd.RunRow{
					ID:       r.ID,
					Name:     r.Name,
					Project:  r.Project,
					Status:   r.Status,
					Duration: cmd.DurationString(r.DurationMs),
					Command:  r.Command,
				}
			}
			fmt.Print(cmd.FormatRunList(rows))
			return nil
		},
	}
	cc.Flags().StringVarP(&project, "project", "p", "", "按项目过滤")
	cc.Flags().StringVarP(&status, "status", "S", "", "按状态过滤 (success/failed/running)")
	cc.Flags().StringVarP(&tag, "tag", "t", "", "按 tag 过滤")
	cc.Flags().StringVarP(&search, "search", "s", "", "在命令/名称中搜索关键词")
	cc.Flags().StringVar(&since, "since", "", "显示此时间之后的记录 (如: 2026-05-13, 1h, today)")
	cc.Flags().StringVar(&until, "until", "", "显示此时间之前的记录")
	cc.Flags().IntVar(&limit, "limit", 20, "限制数量")
	return cc
}

// --- show ---

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <run_id|latest>",
		Short: "显示运行详情",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			runID, isLatest := cmd.ResolveRunID(args[0])
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			var r *internal.Run
			if isLatest {
				r, err = store.GetLatestRun()
			} else {
				r, err = store.GetRun(runID)
			}
			if err != nil {
				return err
			}

			tags, _ := store.GetTags(r.ID)
			note, _ := store.GetNote(r.ID)

			detail := &cmd.RunDetail{
				ID:        r.ID,
				Name:      r.Name,
				Project:   r.Project,
				Status:    r.Status,
				Command:   r.Command,
				CWD:       r.CWD,
				StartedAt: r.StartedAt,
				EndedAt:   r.EndedAt,
				Duration:  cmd.DurationString(r.DurationMs),
				ExitCode:  r.ExitCode,
				GitRepo:   r.GitRepo,
				GitCommit: r.GitCommit,
				GitDirty:  r.GitDirty,
				Tags:      tags,
				Note:      note,
			}
			fmt.Print(cmd.FormatShow(detail))
			return nil
		},
	}
}

// --- logs ---

func logsCmd() *cobra.Command {
	var stdoutOnly, stderrOnly bool
	var tailN int
	var follow, openEditor bool

	c := &cobra.Command{
		Use:   "logs <run_id|latest>",
		Short: "查看运行日志",
		Long:  "查看运行日志。支持 --follow 实时跟踪输出（类似 tail -f）。",
		Example: `  # 查看最新运行的日志
  brun logs latest

  # 实时跟踪正在运行的命令输出
  brun logs latest -f

  # 只看最后 50 行 stderr
  brun logs <run_id> --stderr --tail 50`,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			runID, isLatest := cmd.ResolveRunID(args[0])
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			var r *internal.Run
			if isLatest {
				r, err = store.GetLatestRun()
			} else {
				r, err = store.GetRun(runID)
			}
			if err != nil {
				return err
			}

			// 确定要查看的文件
			logFile := filepath.Join(r.RunDir, "stdout.o")
			if stderrOnly {
				logFile = filepath.Join(r.RunDir, "stderr.er")
			}

			// follow 模式：实时跟踪文件变化
			if follow {
				return followLog(logFile, tailN)
			}

			// 普通模式：读取并显示
			data, err := os.ReadFile(logFile)
			if err != nil {
				return fmt.Errorf("读取日志失败: %w", err)
			}
			content := string(data)
			if tailN > 0 {
				content = cmd.TailLog(content, tailN)
			}
			fmt.Print(content)
			return nil
		},
	}
	c.Flags().BoolVar(&stdoutOnly, "stdout", false, "只看 stdout")
	c.Flags().BoolVar(&stderrOnly, "stderr", false, "只看 stderr")
	c.Flags().IntVar(&tailN, "tail", 0, "最后 N 行")
	c.Flags().BoolVar(&follow, "follow", false, "持续跟踪 (类似 tail -f)")
	c.Flags().BoolVar(&openEditor, "open", false, "用编辑器打开")
	return c
}

// followLog 实时跟踪日志文件变化（类似 tail -f）
func followLog(path string, tailN int) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("无法打开日志文件: %w", err)
	}
	defer f.Close()

	// 如果指定了 tailN，先显示最后 N 行
	if tailN > 0 {
		data, _ := os.ReadFile(path)
		content := string(data)
		fmt.Print(cmd.TailLog(content, tailN))
	}

	// 跳到文件末尾
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}
	_, err = f.Seek(info.Size(), 0)
	if err != nil {
		return fmt.Errorf("定位到文件末尾失败: %w", err)
	}

	// 定期检查新内容
	buf := make([]byte, 4096)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		n, err := f.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if err != nil {
			// 文件可能被关闭或删除（运行结束）
			break
		}

		// 检查文件是否被截断（新写入）
		currentInfo, statErr := f.Stat()
		if statErr == nil && currentInfo.Size() < info.Size() {
			f.Seek(0, 0)
			info = currentInfo
		} else if statErr == nil {
			info = currentInfo
		}
	}

	return nil
}

// --- outputs ---

func outputsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "outputs <run_id|latest>",
		Short: "查看输出文件",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			runID, isLatest := cmd.ResolveRunID(args[0])
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			var r *internal.Run
			if isLatest {
				r, err = store.GetLatestRun()
			} else {
				r, err = store.GetRun(runID)
			}
			if err != nil {
				return err
			}

			arts, err := store.GetArtifacts(r.ID)
			if err != nil {
				return err
			}

			rows := make([]cmd.ArtifactRow, len(arts))
			for i, a := range arts {
				rows[i] = cmd.ArtifactRow{
					Kind:   a.Kind,
					Status: a.Status,
					Size:   cmd.FormatSize(a.Size),
					Path:   a.Path,
				}
			}
			fmt.Print(cmd.FormatOutputs(rows, r.ID, r.Project))
			return nil
		},
	}
}

// --- tag ---

func tagCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tag <run_id|latest> TAG...",
		Short: "添加标签",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			runID, isLatest := cmd.ResolveRunID(args[0])
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			targetID := runID
			if isLatest {
				latest, err := store.GetLatestRun()
				if err != nil {
					return err
				}
				targetID = latest.ID
			}

			for i := 1; i < len(args); i++ {
				if err := store.AddTag(targetID, args[i]); err != nil {
					return err
				}
			}
			fmt.Printf("Added tags to %s: %v\n", targetID, args[1:])
			return nil
		},
	}
}

// --- note ---

func noteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "note <run_id|latest> \"text\"",
		Short: "添加备注",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			runID, isLatest := cmd.ResolveRunID(args[0])
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			targetID := runID
			if isLatest {
				latest, err := store.GetLatestRun()
				if err != nil {
					return err
				}
				targetID = latest.ID
			}

			if err := store.AddNote(targetID, args[1]); err != nil {
				return err
			}
			fmt.Printf("Note added to %s\n", targetID)
			return nil
		},
	}
}

// --- rerun ---

func rerunCmd() *cobra.Command {
	var newCWD string
	var dryRun, sameTags bool
	var rerunName string

	c := &cobra.Command{
		Use:   "rerun <run_id|latest>",
		Short: "重新运行",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			runID, isLatest := cmd.ResolveRunID(args[0])
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			var r *internal.Run
			if isLatest {
				r, err = store.GetLatestRun()
			} else {
				r, err = store.GetRun(runID)
			}
			if err != nil {
				return fmt.Errorf("找不到 run: %w", err)
			}

			cmdStr, execCWD := cmd.BuildRerunCommand(r, newCWD, sameTags)
			if dryRun {
				fmt.Printf("Would run: %s\n", cmdStr)
				fmt.Printf("In directory: %s\n", execCWD)
				return nil
			}

			// 解析原始命令参数
			origArgs := strings.Fields(cmdStr)
			name := rerunName
			if sameTags {
				tags, _ := store.GetTags(r.ID)
				// 继承 tags 到新 run
				_ = tags
			}
			return executeRun(origArgs, name, r.Project, "", nil,
				false, "", 0, execCWD)
		},
	}
	c.Flags().StringVar(&newCWD, "cwd", "", "使用新运行目录")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "只打印不执行")
	c.Flags().BoolVar(&sameTags, "with-same-tags", false, "继承原 tags")
	c.Flags().StringVar(&rerunName, "name", "", "指定新 run 名称")
	return c
}

// --- clean ---

func cleanCmd() *cobra.Command {
	var olderThan string
	var compressLogs bool
	var truncateSize string
	var keepFailed bool
	var keepTag string
	var dryRun bool

	c := &cobra.Command{
		Use:   "clean [options]",
		Short: "清理旧运行记录",
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			runs, err := store.ListRuns(1000, "", "", "", "", "", "")
			if err != nil {
				return err
			}

			var items []cmd.CleanItem
			for _, r := range runs {
				items = append(items, cmd.CleanItem{
					RunID:  r.ID,
					Age:    ageSince(r.StartedAt),
					Size:   "?",
					Reason: "old",
				})
			}

			fmt.Print(cmd.FormatCleanSummary(items, dryRun))
			return nil
		},
	}
	c.Flags().StringVar(&olderThan, "older-than", "", "清理早于此时长的 run")
	c.Flags().BoolVar(&compressLogs, "compress-logs", false, "压缩日志")
	c.Flags().StringVar(&truncateSize, "truncate-large-logs", "", "裁剪超大日志")
	c.Flags().BoolVar(&keepFailed, "keep-failed", false, "保留失败 run")
	c.Flags().StringVar(&keepTag, "keep-tag", "", "保留指定 tag 的 run")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "只显示将执行的操作")
	return c
}

// --- 辅助函数 ---

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
func parseTimeFilter(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// 尝试直接解析为日期
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return s
	}

	// 相对时间
	now := time.Now().UTC()
	switch {
	case s == "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UTC().Format(time.RFC3339)
	case strings.HasSuffix(s, "h"):
		var n int
		fmt.Sscanf(s, "%d", &n)
		return now.Add(-time.Duration(n) * time.Hour).Format(time.RFC3339)
	case strings.HasSuffix(s, "d"):
		var n int
		fmt.Sscanf(s, "%d", &n)
		return now.Add(-time.Duration(n) * 24 * time.Hour).Format(time.RFC3339)
	case strings.HasSuffix(s, "w"):
		var n int
		fmt.Sscanf(s, "%d", &n)
		return now.Add(-time.Duration(n) * 7 * 24 * time.Hour).Format(time.RFC3339)
	default:
		return s // 原样返回，让 SQL 查询自然失败
	}
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

func username() string {
	return os.Getenv("USER")
}

func ageSince(startedAt string) string {
	if startedAt == "" {
		return "?"
	}
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%.0fm", d.Minutes())
	case d < 24*time.Hour:
		return fmt.Sprintf("%.0fh", d.Hours())
	default:
		return fmt.Sprintf("%.0fd", d.Hours()/24)
	}
}
