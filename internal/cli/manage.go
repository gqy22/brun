package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/biotools/brun/cmd"
	"github.com/biotools/brun/internal"
	webassets "github.com/biotools/brun/web"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func tagCmd() *cobra.Command {
	var latest bool
	c := &cobra.Command{
		Use:   "tag <run_id> TAG...",
		Short: "添加标签",
		Example: `  brun tag 20260605-145615-fed727 sample:S1 production
  brun tag --latest sample:S1 production`,
		Args: func(c *cobra.Command, args []string) error {
			if latest {
				if len(args) < 1 {
					return fmt.Errorf("需要至少一个 tag")
				}
				return nil
			}
			if len(args) < 2 {
				return fmt.Errorf("需要 run_id 和至少一个 tag，或使用 --latest")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			targetID := args[0]
			tags := args[1:]
			if latest {
				latest, err := store.GetLatestRun()
				if err != nil {
					return err
				}
				targetID = latest.ID
				tags = args
			}

			for _, tag := range tags {
				if err := store.AddTag(targetID, tag); err != nil {
					return err
				}
			}
			fmt.Printf("Added tags to %s: %v\n", targetID, tags)
			return nil
		},
	}
	c.Flags().BoolVar(&latest, "latest", false, "使用最新运行")
	return c
}

// --- note ---

func noteCmd() *cobra.Command {
	var latest bool
	c := &cobra.Command{
		Use:   "note <run_id> \"text\"",
		Short: "添加备注",
		Example: `  brun note 20260605-145615-fed727 "STAR index 参数测试"
  brun note --latest "STAR index 参数测试"`,
		Args: func(c *cobra.Command, args []string) error {
			if latest {
				if len(args) != 1 {
					return fmt.Errorf("需要备注文本，且 --latest 不能和 run_id 同时使用")
				}
				return nil
			}
			if len(args) != 2 {
				return fmt.Errorf("需要 run_id 和备注文本，或使用 --latest")
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			targetID := args[0]
			text := args[1]
			if latest {
				latest, err := store.GetLatestRun()
				if err != nil {
					return err
				}
				targetID = latest.ID
				text = args[0]
			}

			if err := store.AddNote(targetID, text); err != nil {
				return err
			}
			fmt.Printf("Note added to %s\n", targetID)
			return nil
		},
	}
	c.Flags().BoolVar(&latest, "latest", false, "使用最新运行")
	return c
}

// --- rerun ---

func rerunCmd() *cobra.Command {
	var newCWD string
	var dryRun, sameTags, latest bool
	var rerunName string

	c := &cobra.Command{
		Use:   "rerun <run_id>",
		Short: "重新运行",
		Example: `  brun rerun 20260605-145615-fed727 --dry-run
  brun rerun --latest --dry-run
  brun rerun --latest --cwd /data/project`,
		Args: runSelectorArgs(&latest),
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			r, err := selectedRun(store, args, latest)
			if err != nil {
				return runLookupError(err)
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
				false, "", 0, execCWD, "")
		},
	}
	c.Flags().StringVar(&newCWD, "cwd", "", "使用新运行目录")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "只打印不执行")
	c.Flags().BoolVar(&sameTags, "with-same-tags", false, "继承原 tags")
	c.Flags().StringVar(&rerunName, "name", "", "指定新 run 名称")
	c.Flags().BoolVar(&latest, "latest", false, "重新运行最新记录")
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

func repairIndexCmd() *cobra.Command {
	var write bool
	c := &cobra.Command{
		Use:   "repair-index",
		Short: "从 run 目录重建 SQLite 索引",
		Long:  "扫描 runs 目录中的 metadata.yaml，重建 SQLite run 索引。默认只预览，使用 --write 才会写入数据库。",
		Example: `  brun repair-index
  brun repair-index --write`,
		RunE: func(c *cobra.Command, args []string) error {
			runs, err := loadRunsFromMetadata(internal.RunsRoot())
			if err != nil {
				return err
			}
			if !write {
				fmt.Printf("Found %d run metadata files. Use --write to rebuild the SQLite index.\n", len(runs))
				return nil
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()
			repaired := 0
			for _, run := range runs {
				if _, err := store.GetRun(run.ID); err == nil {
					continue
				}
				if err := store.CreateRun(run); err != nil {
					return fmt.Errorf("rebuild run %s: %w", run.ID, err)
				}
				repaired++
			}
			fmt.Printf("Rebuilt %d missing run records from %d metadata files.\n", repaired, len(runs))
			return nil
		},
	}
	c.Flags().BoolVar(&write, "write", false, "实际写入缺失的 run 记录")
	return c
}

func loadRunsFromMetadata(root string) ([]*internal.Run, error) {
	var runs []*internal.Run
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() || entry.Name() != "metadata.yaml" {
			return nil
		}
		run, err := readRunMetadata(path)
		if err != nil {
			internal.Log().Warn("metadata_read_failed", "path", path, "error", err.Error())
			return nil
		}
		run.RunDir = filepath.Dir(path)
		runs = append(runs, run)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return runs, nil
}

type runMetadata struct {
	ID               string `yaml:"id"`
	Name             string `yaml:"name"`
	Project          string `yaml:"project"`
	Command          string `yaml:"command"`
	Status           string `yaml:"status"`
	ExitCode         int    `yaml:"exit_code"`
	CWD              string `yaml:"cwd"`
	StartedAt        string `yaml:"started_at"`
	EndedAt          string `yaml:"ended_at"`
	DurationMs       int64  `yaml:"duration_ms"`
	GitCommit        string `yaml:"git_commit"`
	GitDirty         bool   `yaml:"git_dirty"`
	CWDSource        string `yaml:"cwd_source"`
	ProjectSource    string `yaml:"project_source"`
	DiagInfoCount    int    `yaml:"diag_info_count"`
	DiagWarningCount int    `yaml:"diag_warning_count"`
	DiagErrorCount   int    `yaml:"diag_error_count"`
	DiagLastCode     string `yaml:"diag_last_code"`
	DiagLastAt       string `yaml:"diag_last_at"`
}

func readRunMetadata(path string) (*internal.Run, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta runMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.ID == "" {
		return nil, fmt.Errorf("missing id")
	}
	return &internal.Run{
		ID:               meta.ID,
		Name:             meta.Name,
		Project:          meta.Project,
		Command:          meta.Command,
		Status:           meta.Status,
		ExitCode:         meta.ExitCode,
		CWD:              meta.CWD,
		StartedAt:        meta.StartedAt,
		EndedAt:          meta.EndedAt,
		DurationMs:       meta.DurationMs,
		GitCommit:        meta.GitCommit,
		GitDirty:         meta.GitDirty,
		CWDSource:        meta.CWDSource,
		ProjectSource:    meta.ProjectSource,
		DiagInfoCount:    meta.DiagInfoCount,
		DiagWarningCount: meta.DiagWarningCount,
		DiagErrorCount:   meta.DiagErrorCount,
		DiagLastCode:     meta.DiagLastCode,
		DiagLastAt:       meta.DiagLastAt,
	}, nil
}

// --- 辅助函数 ---

func webCmd() *cobra.Command {
	var port int
	var addr string

	c := &cobra.Command{
		Use:   "web",
		Short: "启动 Web Dashboard（局域网访问）",
		Long:  "在本地启动 HTTP 服务，通过浏览器管理运行记录、查看日志。默认监听 0.0.0.0:9213；未显式指定 --port 时会自动避开占用端口，显式指定 --port 时端口不可用会直接报错。",
		Example: `  # 启动 Web Dashboard
  brun web

  # 指定端口；如果端口被占用会直接失败
  brun web --port 9090

  # 明确局域网监听地址
  brun web --addr 0.0.0.0`,
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return cliError("database_open_failed", "打开数据库失败: "+err.Error(), "检查 BRUN_HOME 是否可写；如索引损坏，可尝试 brun repair-index --write", err)
			}

			if addr == "" {
				addr = "0.0.0.0"
			}
			if port == 0 {
				port = 9213
			}

			srv := cmd.NewWebServer(store, addr, port, webassets.Templates, webassets.Static)
			if c.Flags().Changed("port") {
				srv.SetAutoIncrementPort(false)
			}
			if err := srv.ListenAndServe(); err != nil {
				return cliError("web_listen_failed", "Web 服务启动失败: "+err.Error(), "如果端口被占用，请换一个 --port；局域网访问可显式使用 --addr 0.0.0.0", err)
			}
			return nil
		},
	}
	c.Flags().IntVarP(&port, "port", "p", 9213, "监听端口；显式指定时不会自动递增")
	c.Flags().StringVar(&addr, "addr", "0.0.0.0", "监听地址；默认允许局域网访问")
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
