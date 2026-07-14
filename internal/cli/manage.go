package cli

import (
	"encoding/json"
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
	ht := MustParse("tag")
	c := &cobra.Command{
		Args: func(c *cobra.Command, args []string) error {
			if latest {
				if len(args) < 1 {
					return cliError("missing_tag", "需要至少一个标签", "例如 brun tag --latest sample:S1", nil)
				}
				return nil
			}
			if len(args) < 2 {
				return cliError("missing_tag_target", "需要 run_id 和至少一个标签，或使用 --latest", "例如 brun tag <run_id> sample:S1", nil)
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			var target *internal.Run
			tags := args[1:]
			if latest {
				target, err = store.GetLatestRun()
				tags = args
			} else {
				target, err = store.GetRun(args[0])
			}
			if err != nil {
				return runLookupError(err)
			}

			for _, tag := range tags {
				if err := store.AddTag(target.ID, tag); err != nil {
					return err
				}
			}
			fmt.Printf("已为 %s 添加标签: %v\n", target.ID, tags)
			return nil
		},
	}
	c.Flags().BoolVar(&latest, "latest", false, "使用最新运行")
	ht.Inject(c)
	return c
}

// --- note ---

func noteCmd() *cobra.Command {
	var latest bool
	ht := MustParse("note")
	c := &cobra.Command{
		Args: func(c *cobra.Command, args []string) error {
			if latest {
				if len(args) != 1 {
					return cliError("invalid_note_args", "--latest 模式需要且只能指定备注文本", "例如 brun note --latest \"备注\"", nil)
				}
				return nil
			}
			if len(args) != 2 {
				return cliError("invalid_note_args", "需要 run_id 和备注文本，或使用 --latest", "例如 brun note <run_id> \"备注\"", nil)
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			var target *internal.Run
			text := args[1]
			if latest {
				target, err = store.GetLatestRun()
				text = args[0]
			} else {
				target, err = store.GetRun(args[0])
			}
			if err != nil {
				return runLookupError(err)
			}

			if err := store.AddNote(target.ID, text); err != nil {
				return err
			}
			fmt.Printf("已为 %s 更新备注\n", target.ID)
			return nil
		},
	}
	c.Flags().BoolVar(&latest, "latest", false, "使用最新运行")
	ht.Inject(c)
	return c
}

// --- stop ---

func stopCmd() *cobra.Command {
	var latest bool
	var force bool

	ht := MustParse("stop")
	c := &cobra.Command{
		Args: runSelectorArgs(&latest),
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			run, err := selectedRun(store, args, latest)
			if err != nil {
				return runLookupError(err)
			}

			if run.Status != "running" {
				return cliError("stop_not_running",
					fmt.Sprintf("任务 %s 当前状态为 %s，无法终止", run.ID, run.Status),
					"只能终止状态为 running 的任务；使用 brun list 查看运行中的任务", nil)
			}

			pidFile := filepath.Join(run.RunDir, ".pid")
			data, err := os.ReadFile(pidFile)
			if err != nil {
				if os.IsNotExist(err) {
					_ = store.UpdateRunStatus(run.ID, "failed", -1, "", 0)
					return cliError("stop_no_pidfile",
						fmt.Sprintf("找不到 %s 的进程信息（PID 文件不存在）", run.ID),
						"任务可能已经自行结束；使用 brun show "+run.ID+" 确认当前状态", nil)
				}
				return cliError("stop_read_pidfile",
					fmt.Sprintf("读取 PID 文件失败: %v", err),
					"检查 run 目录权限: "+run.RunDir, err)
			}

			var pid int
			if _, scanErr := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); scanErr != nil || pid <= 0 {
				return cliError("stop_invalid_pid",
					fmt.Sprintf("无效的 PID: %q", strings.TrimSpace(string(data))),
					"PID 文件可能损坏，建议确认任务状态后手动处理", nil)
			}

			// 记录最终资源数据
			pss, cst := cmd.ReadProcStats(pid)
			if pss > 0 || cst > 0 {
				_ = store.UpdateRunResources(run.ID, pss, cst, cmd.ResourceSupported(), "ok")
			}

			fmt.Printf("正在终止任务 %s (PID: %d, 命令: %s)...\n", run.ID, pid, run.Command)

			// 调用统一的 StopRun，3 秒宽限期
			result := cmd.StopRun(pid, 3, force)

			if result.AlreadyDead {
				_ = store.UpdateRunStatus(run.ID, "failed", -1, "", 0)
				fmt.Printf("%s\n", result.Msg)
				return nil
			}

			if !result.OK {
				return cliError("stop_failed", result.Msg, "检查进程状态和权限；可尝试 brun stop "+run.ID+" --force", nil)
			}

			fmt.Printf("%s\n", result.Msg)
			return nil
		},
	}
	c.Flags().BoolVar(&latest, "latest", false, "终止最新运行中的任务")
	c.Flags().BoolVarP(&force, "force", "f", false, "跳过宽限期直接强制终止 (SIGKILL)")
	ht.Inject(c)
	return c
}

// --- rerun ---

func rerunCmd() *cobra.Command {
	var newCWD string
	var dryRun, sameTags, latest bool
	var rerunName string

	ht := MustParse("rerun")
	c := &cobra.Command{
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
				fmt.Printf("将执行: %s\n", cmdStr)
				fmt.Printf("运行目录: %s\n", execCWD)
				return nil
			}

			name := rerunName
			var tags []string
			if sameTags {
				tags, err = store.GetTags(r.ID)
				if err != nil {
					return err
				}
			}
			return executeRun(cmd.ShellCommandArgs(cmdStr), name, r.Project, "", tags,
				false, "", 0, execCWD, "")
		},
	}
	c.Flags().StringVar(&newCWD, "cwd", "", "使用新运行目录")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "只打印不执行")
	c.Flags().BoolVar(&sameTags, "with-same-tags", false, "继承原 tags")
	c.Flags().StringVarP(&rerunName, "name", "n", "", "指定新 run 名称")
	c.Flags().BoolVar(&latest, "latest", false, "重新运行最新记录")
	ht.Inject(c)
	return c
}

// --- clean ---

func cleanCmd() *cobra.Command {
	var olderThan string
	var keepFailed bool
	var keepTag string
	var write bool
	var jsonOut bool

	ht := MustParse("clean")
	c := &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if olderThan == "" {
				return cliError("missing_clean_filter", "缺少清理条件 --older-than", "例如 brun clean --older-than 30d；默认只预览，实际删除需加 --write", nil)
			}
			thresholdValue, err := parseTimeFilter(olderThan)
			if err != nil {
				return cliError("invalid_clean_time", "--older-than 无效: "+err.Error(), "支持 YYYY-MM-DD、RFC3339、today、Nh、Nd、Nw", err)
			}
			threshold, err := time.Parse(time.RFC3339, thresholdValue)
			if err != nil {
				return err
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			runs, err := store.ListRuns(-1, "", "", "", "", "", "", false, "", "")
			if err != nil {
				return err
			}

			items := make([]cmd.CleanItem, 0)
			for _, r := range runs {
				if r.Status == "running" {
					continue
				}
				if keepFailed && r.Status == "failed" {
					continue
				}
				if keepTag != "" {
					tags, err := store.GetTags(r.ID)
					if err != nil {
						return err
					}
					if containsString(tags, keepTag) {
						continue
					}
				}
				age, oldEnough, err := runOlderThan(r.StartedAt, threshold)
				if err != nil || !oldEnough {
					continue
				}
				items = append(items, cmd.CleanItem{
					RunID:  r.ID,
					Age:    age,
					Size:   formatDirSize(r.RunDir),
					Reason: "older_than=" + olderThan,
				})
			}

			if jsonOut {
				payload := map[string]any{
					"write":       write,
					"older_than":  olderThan,
					"keep_failed": keepFailed,
					"keep_tag":    keepTag,
					"count":       len(items),
					"runs":        items,
				}
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(payload); err != nil {
					return err
				}
			} else {
				fmt.Fprint(c.OutOrStdout(), cmd.FormatCleanSummary(items, !write))
			}
			if !write {
				return nil
			}
			for _, item := range items {
				if err := store.DeleteRun(item.RunID); err != nil {
					return cliError("clean_delete_failed", "删除 run 失败: "+err.Error(), "检查 run 目录权限和 SQLite 状态；可重新运行 brun clean --older-than "+olderThan, err)
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&olderThan, "older-than", "", "清理早于指定时间阈值的 run")
	c.Flags().BoolVar(&keepFailed, "keep-failed", false, "保留失败 run")
	c.Flags().StringVar(&keepTag, "keep-tag", "", "保留指定 tag 的 run")
	c.Flags().BoolVar(&write, "write", false, "实际删除匹配的 run；不传时只预览")
	c.Flags().BoolVar(&jsonOut, "json", false, "输出 JSON")
	ht.Inject(c)
	return c
}

func repairCmd() *cobra.Command {
	var write bool
	ht := MustParse("repair")
	c := &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			runs, err := loadRunsFromMetadata(internal.RunsRoot())
			if err != nil {
				return err
			}
			if !write {
				fmt.Printf("发现 %d 个 run 元数据文件。使用 --write 重建 SQLite 索引。\n", len(runs))
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
			fmt.Printf("已从 %d 个元数据文件中重建 %d 条缺失运行记录。\n", len(runs), repaired)
			return nil
		},
	}
	c.Flags().BoolVar(&write, "write", false, "实际写入缺失的 run 记录")
	ht.Inject(c)
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
	ID                string `yaml:"id"`
	Name              string `yaml:"name"`
	Project           string `yaml:"project"`
	Command           string `yaml:"command"`
	Status            string `yaml:"status"`
	ExitCode          int    `yaml:"exit_code"`
	CWD               string `yaml:"cwd"`
	StartedAt         string `yaml:"started_at"`
	EndedAt           string `yaml:"ended_at"`
	DurationMs        int64  `yaml:"duration_ms"`
	GitCommit         string `yaml:"git_commit"`
	GitDirty          bool   `yaml:"git_dirty"`
	Hostname          string `yaml:"hostname"`
	HostnameStatus    string `yaml:"hostname_status"`
	Username          string `yaml:"username"`
	UsernameStatus    string `yaml:"username_status"`
	CondaStatus       string `yaml:"conda_status"`
	CondaEnv          string `yaml:"conda_env"`
	CondaPrefix       string `yaml:"conda_prefix"`
	PythonVersion     string `yaml:"python_version"`
	ResourceSupported bool   `yaml:"resource_supported"`
	ResourceStatus    string `yaml:"resource_status"`
	PeakRSSKB         int64  `yaml:"peak_rss_kb"`
	CPUTimeMs         int64  `yaml:"cpu_time_ms"`
	CWDSource         string `yaml:"cwd_source"`
	ProjectSource     string `yaml:"project_source"`
	DiagInfoCount     int    `yaml:"diag_info_count"`
	DiagWarningCount  int    `yaml:"diag_warning_count"`
	DiagErrorCount    int    `yaml:"diag_error_count"`
	DiagLastCode      string `yaml:"diag_last_code"`
	DiagLastAt        string `yaml:"diag_last_at"`
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
		ID:                meta.ID,
		Name:              meta.Name,
		Project:           meta.Project,
		Command:           meta.Command,
		Status:            meta.Status,
		ExitCode:          meta.ExitCode,
		CWD:               meta.CWD,
		StartedAt:         meta.StartedAt,
		EndedAt:           meta.EndedAt,
		DurationMs:        meta.DurationMs,
		GitCommit:         meta.GitCommit,
		GitDirty:          meta.GitDirty,
		Hostname:          meta.Hostname,
		HostnameStatus:    meta.HostnameStatus,
		Username:          meta.Username,
		UsernameStatus:    meta.UsernameStatus,
		CondaStatus:       meta.CondaStatus,
		CondaEnv:          meta.CondaEnv,
		CondaPrefix:       meta.CondaPrefix,
		PythonVersion:     meta.PythonVersion,
		ResourceSupported: meta.ResourceSupported,
		ResourceStatus:    meta.ResourceStatus,
		PeakRSSKB:         meta.PeakRSSKB,
		CPUTimeMs:         meta.CPUTimeMs,
		CWDSource:         meta.CWDSource,
		ProjectSource:     meta.ProjectSource,
		DiagInfoCount:     meta.DiagInfoCount,
		DiagWarningCount:  meta.DiagWarningCount,
		DiagErrorCount:    meta.DiagErrorCount,
		DiagLastCode:      meta.DiagLastCode,
		DiagLastAt:        meta.DiagLastAt,
	}, nil
}

// --- 辅助函数 ---

func webCmd() *cobra.Command {
	var port int
	var addr string

	ht := MustParse("web")
	c := &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return cliError("database_open_failed", "打开数据库失败: "+err.Error(), "检查 BRUN_HOME 是否可写；如索引损坏，可尝试 brun repair --write", err)
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
	ht.Inject(c)
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

func runOlderThan(startedAt string, threshold time.Time) (string, bool, error) {
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return "?", false, err
	}
	age := time.Since(t)
	if age < 0 {
		age = 0
	}
	return humanCleanAge(age), !t.After(threshold), nil
}

func humanCleanAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func formatDirSize(path string) string {
	size, err := dirSize(path)
	if err != nil {
		return "?"
	}
	return cmd.FormatSize(size)
}

func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
