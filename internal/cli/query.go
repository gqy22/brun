package cli

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

func listCmd() *cobra.Command {
	var project, status, tag, search, since, until string
	var limit int

	cc := &cobra.Command{
		Use:   "list",
		Short: "列出运行历史",
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStoreReadOnly()
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
				diag := diagnosticSummaryLabel(r.RunDir)
				rows[i] = cmd.RunRow{
					ID:         r.ID,
					Name:       r.Name,
					Project:    r.Project,
					Status:     r.Status,
					Diagnostic: diag,
					Duration:   cmd.DisplayDuration(r.Status, r.StartedAt, r.DurationMs),
					Command:    r.Command,
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
				Duration:  cmd.DisplayDuration(r.Status, r.StartedAt, r.DurationMs),
				ExitCode:  r.ExitCode,
				PeakRSSKB: r.PeakRSSKB,
				CPUTimeMs: r.CPUTimeMs,
				GitRepo:   r.GitRepo,
				GitCommit: r.GitCommit,
				GitDirty:  r.GitDirty,
				Tags:      tags,
				Note:      note,
				Diag:      toCmdDiagnosticDetail(readDiagnosticSummaryQuiet(r.RunDir)),
			}
			fmt.Print(cmd.FormatShow(detail))
			return nil
		},
	}
}

func diagnosticSummaryLabel(runDir string) string {
	summary := readDiagnosticSummaryQuiet(runDir)
	if summary.ErrorCount > 0 {
		return fmt.Sprintf("E%d", summary.ErrorCount)
	}
	if summary.WarningCount > 0 {
		return fmt.Sprintf("W%d", summary.WarningCount)
	}
	return "-"
}

func readDiagnosticSummaryQuiet(runDir string) internal.DiagnosticSummary {
	summary, err := internal.ReadDiagnosticSummary(runDir)
	if err != nil {
		internal.Log().Warn("diagnostic_summary_read_failed", "run_dir", runDir, "error", err.Error())
		return internal.DiagnosticSummary{}
	}
	return summary
}

func toCmdDiagnosticDetail(summary internal.DiagnosticSummary) cmd.DiagnosticDetail {
	return cmd.DiagnosticDetail{
		InfoCount:    summary.InfoCount,
		WarningCount: summary.WarningCount,
		ErrorCount:   summary.ErrorCount,
		LastLevel:    summary.LastLevel,
		LastCode:     summary.LastCode,
		LastMessage:  summary.LastMessage,
		LastAt:       summary.LastAt,
	}
}

// --- script ---

func scriptCmd() *cobra.Command {
	var pathOnly bool

	c := &cobra.Command{
		Use:   "script <run_id|latest>",
		Short: "查看运行时保存的脚本快照",
		Example: `  brun script latest
  brun script 20260522-153012-a8f3c2
  brun script latest --path`,
		Args: cobra.ExactArgs(1),
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

			snapshot, err := cmd.ReadScriptSnapshot(r.RunDir)
			if err != nil {
				return err
			}
			if pathOnly {
				fmt.Fprintln(c.OutOrStdout(), snapshot.Path)
				return nil
			}
			fmt.Fprint(c.OutOrStdout(), snapshot.Content)
			return nil
		},
	}
	c.Flags().BoolVar(&pathOnly, "path", false, "只输出脚本快照路径")
	return c
}

// --- logs ---

func logsCmd() *cobra.Command {
	var stdoutOnly, stderrOnly bool
	var tailN int
	var follow bool

	c := &cobra.Command{
		Use:   "logs [run_id]",
		Short: "查看运行日志",
		Long:  "查看运行日志。支持 --follow 实时跟踪输出（类似 tail -f）。不传参数默认查看最新运行。",
		Example: `  # 查看最新运行的日志 (默认)
  brun logs
  brun logs latest

  # 实时跟踪正在运行的命令输出
  brun logs -f

  # 只看最后 50 行 stderr
  brun logs <run_id> --stderr --tail 50`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			arg := "latest"
			if len(args) > 0 {
				arg = args[0]
			}
			runID, isLatest := cmd.ResolveRunID(arg)
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
			// 未指定 --stdout/--stderr 时，默认同时显示两者
			if !stdoutOnly && !stderrOnly {
				stdoutPath := filepath.Join(r.RunDir, "stdout.o")
				stderrPath := filepath.Join(r.RunDir, "stderr.er")
				printLogSection("stdout", stdoutPath, tailN)
				printLogSection("stderr", stderrPath, tailN)
				return nil
			}

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

func printLogSection(label, path string, tailN int) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("[%s] (文件不存在)\n", label)
		return
	}
	content := string(data)
	if tailN > 0 {
		content = cmd.TailLog(content, tailN)
	}
	if content == "" {
		fmt.Printf("[%s] (空)\n", label)
		return
	}
	fmt.Printf("=== %s ===\n", label)
	fmt.Print(content)
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

// --- web ---
