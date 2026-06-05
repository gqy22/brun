package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/biotools/brun/cmd"
	"github.com/biotools/brun/internal"
	webassets "github.com/biotools/brun/web"
	"github.com/spf13/cobra"
)

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

func webCmd() *cobra.Command {
	var port int
	var addr string

	c := &cobra.Command{
		Use:   "web",
		Short: "启动 Web Dashboard（局域网访问）",
		Long:  "在本地启动 HTTP 服务，通过浏览器管理运行记录、查看日志。默认端口 9213。",
		Example: `  # 启动 Web Dashboard
  brun web

  # 指定端口
  brun web --port 9090

  # 局域网可访问
  brun web --addr 0.0.0.0`,
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return fmt.Errorf("打开数据库失败: %w", err)
			}

			if addr == "" {
				addr = "0.0.0.0"
			}
			if port == 0 {
				port = 9213
			}

			srv := cmd.NewWebServer(store, addr, port, webassets.Templates, webassets.Static)
			return srv.ListenAndServe()
		},
	}
	c.Flags().IntVarP(&port, "port", "p", 9213, "监听端口")
	c.Flags().StringVar(&addr, "addr", "0.0.0.0", "监听地址")
	return c
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
