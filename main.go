package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/biotools/brun/cmd"
	"github.com/biotools/brun/internal"
	"github.com/spf13/cobra"
)

var version = "0.1.0-dev"

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

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func openStore() (*internal.Store, error) {
	return internal.NewStore(filepath.Join(internal.HomeDir(), "db.sqlite"))
}

// --- init ---

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "在当前目录生成 brun.yaml",
		RunE: func(c *cobra.Command, args []string) error {
			project := ""
			if len(args) > 0 {
				project = args[0]
			}
			return cmd.WriteInitFile(".", project)
		},
	}
}

// --- run (核心编排) ---

func runCmd() *cobra.Command {
	var name, project, note string
	var tags, inputs, outputs []string
	var noFsDiff, hashOutputs, copyOutputs bool
	var timeout int
	var cwdFlag string

	c := &cobra.Command{
		Use:   "run -- <command...>",
		Short: "执行命令并记录运行信息",
		RunE: func(c *cobra.Command, args []string) error {
			return executeRun(args, name, project, note, tags, inputs, outputs,
				noFsDiff, hashOutputs, copyOutputs, time.Duration(timeout)*time.Second, cwdFlag)
		},
	}
	c.Flags().StringVar(&name, "name", "", "run 名称")
	c.Flags().StringVar(&project, "project", "", "项目名")
	c.Flags().StringVar(&note, "note", "", "备注")
	c.Flags().StringArrayVar(&tags, "tag", []string{}, "标签")
	c.Flags().StringArrayVar(&inputs, "input", []string{}, "输入文件")
	c.Flags().StringArrayVar(&outputs, "output", []string{}, "输出文件")
	c.Flags().BoolVar(&noFsDiff, "no-fs-diff", false, "禁用文件系统 diff")
	c.Flags().BoolVar(&hashOutputs, "hash-outputs", false, "计算输出文件 hash")
	c.Flags().BoolVar(&copyOutputs, "copy-outputs", false, "复制输出文件")
	c.Flags().IntVar(&timeout, "timeout", 0, "超时(秒)")
	c.Flags().StringVar(&cwdFlag, "cwd", "", "运行目录")
	return c
}

func executeRun(args []string, name, project, note string, tags, inputs, outputs []string,
	noFsDiff, hashOutputs, copyOutputs bool, timeout time.Duration, cwdFlag string) error {

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

	// 10. 执行命令
	stdoutPath := filepath.Join(runDir, "stdout.log")
	stderrPath := filepath.Join(runDir, "stderr.log")
	result := cmd.ExecuteCommand(args, cwd, stdoutPath, stderrPath, timeout)

	// 11. after 快照 + diff
	if !noFsDiff && len(beforeSnapshot) > 0 {
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

	// 12. 处理显式声明的 outputs
	for _, out := range outputs {
		store.CreateArtifact(&internal.Artifact{
			RunID:  runID,
			Kind:   "output",
			Status: "declared",
			Path:   out,
		})
	}

	// 13. 处理 tags 和 note
	for _, t := range tags {
		store.AddTag(runID, t)
	}
	if note != "" {
		store.AddNote(runID, note)
	}

	// 14. 更新 DB status
	if err := store.UpdateRunStatus(runID, result.Status, result.ExitCode,
		result.EndedAt, result.DurationMs); err != nil {
		return fmt.Errorf("更新状态失败: %w", err)
	}

	// 15. 更新 runRecord 并写 metadata.yaml
	runRecord.Status = result.Status
	runRecord.ExitCode = result.ExitCode
	runRecord.EndedAt = result.EndedAt
	runRecord.DurationMs = result.DurationMs
	metaYAML := cmd.BuildMetadataYAML(runRecord)
	os.WriteFile(filepath.Join(runDir, "metadata.yaml"), []byte(metaYAML), 0644)

	// 16. 打印摘要
	fmt.Printf("\nCommand finished %s in %s\n", result.Status, cmd.DurationString(result.DurationMs))
	arts, _ := store.GetArtifacts(runID)
	if len(arts) > 0 {
		fmt.Printf("Outputs detected: %d\n", len(arts))
	}

	return nil
}

// --- list ---

func listCmd() *cobra.Command {
	var project, status, tag string
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

			runs, err := store.ListRuns(limit, project, status, tag)
			if err != nil {
				return err
			}

			rows := make([]cmd.RunRow, len(runs))
			for i, r := range runs {
				rows[i] = cmd.RunRow{
					ID:       r.ID,
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
	cc.Flags().StringVar(&project, "project", "", "按项目过滤")
	cc.Flags().StringVar(&status, "status", "", "按状态过滤 (success/failed/running)")
	cc.Flags().StringVar(&tag, "tag", "", "按 tag 过滤")
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

			if stdoutOnly {
				data, _ := os.ReadFile(filepath.Join(r.RunDir, "stdout.log"))
				content := string(data)
				if tailN > 0 {
					content = cmd.TailLog(content, tailN)
				}
				fmt.Print(content)
				return nil
			}
			if stderrOnly {
				data, _ := os.ReadFile(filepath.Join(r.RunDir, "stderr.log"))
				content := string(data)
				if tailN > 0 {
					content = cmd.TailLog(content, tailN)
				}
				fmt.Print(content)
				return nil
			}

			// 默认显示 stdout
			data, _ := os.ReadFile(filepath.Join(r.RunDir, "stdout.log"))
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
	c.Flags().BoolVar(&follow, "follow", false, "持续跟踪")
	c.Flags().BoolVar(&openEditor, "open", false, "用编辑器打开")
	return c
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
			return executeRun(origArgs, name, r.Project, "", nil, nil, nil,
				false, false, false, 0, execCWD)
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

			runs, err := store.ListRuns(1000, "", "", "")
			if err != nil {
				return err
			}

			var items []cmd.CleanItem
			for _, r := range runs {
				items = append(items, cmd.CleanItem{
					RunID: r.ID,
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
