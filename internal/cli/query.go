package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/biotools/brun/cmd"
	"github.com/biotools/brun/internal"
	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	var project, status, tag, search, since, until, host, user string
	var limit int
	var jsonOutput bool

	ht := MustParse("list")
	cc := &cobra.Command{
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if limit <= 0 {
				return cliError("invalid_limit", "--limit 必须大于 0", "例如 brun list --limit 20", nil)
			}
			store, err := openStoreReadOnly()
			if err != nil {
				return err
			}
			defer store.Close()

			sinceVal, untilVal := since, until
			if since != "" {
				var parseErr error
				sinceVal, parseErr = parseTimeFilter(since)
				if parseErr != nil {
					return cliError("invalid_time_filter", "--since "+parseErr.Error(), "使用 YYYY-MM-DD、RFC3339、today、Nh、Nd、Nw", parseErr)
				}
			}
			if until != "" {
				var parseErr error
				untilVal, parseErr = parseTimeFilter(until)
				if parseErr != nil {
					return cliError("invalid_time_filter", "--until "+parseErr.Error(), "使用 YYYY-MM-DD、RFC3339、today、Nh、Nd、Nw", parseErr)
				}
			}

			baseStatus, withWarnings, err := parseListStatusFilter(status)
			if err != nil {
				return err
			}
			runs, err := store.ListRuns(limit, project, baseStatus, tag, search, sinceVal, untilVal, withWarnings, host, user)
			if err != nil {
				return err
			}

			if jsonOutput {
				payload := make([]runJSONPayload, len(runs))
				for i, r := range runs {
					payload[i] = runJSON(r, nil, "")
				}
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}

			rows := make([]cmd.RunRow, len(runs))
			for i, r := range runs {
				diag := diagnosticSummaryLabel(r)
				rows[i] = cmd.RunRow{
					ID:            r.ID,
					Name:          r.Name,
					Project:       r.Project,
					Status:        r.Status,
					DisplayStatus: r.DisplayStatus(),
					Diagnostic:    diag,
					Duration:      cmd.DisplayDuration(r.Status, r.StartedAt, r.DurationMs),
					Command:       r.Command,
					CWD:           r.CWD,
				}
			}
			fmt.Print(cmd.FormatRunList(rows))
			return nil
		},
	}
	cc.Flags().StringVarP(&project, "project", "p", "", "按项目过滤")
	cc.Flags().StringVarP(&status, "status", "S", "", "按状态过滤 (running/success/failed/cancelled 及 *_with_warnings)")
	cc.Flags().StringVarP(&tag, "tag", "t", "", "按 tag 过滤")
	cc.Flags().StringVarP(&search, "search", "s", "", "在命令/名称中搜索关键词")
	cc.Flags().StringVar(&since, "since", "", "显示此时间之后的记录 (YYYY-MM-DD, RFC3339, today, Nh/Nd/Nw)")
	cc.Flags().StringVar(&until, "until", "", "显示此时间之前的记录 (YYYY-MM-DD, RFC3339, today, Nh/Nd/Nw)")
	cc.Flags().StringVar(&host, "host", "", "按 hostname 过滤 (精确匹配)")
	cc.Flags().StringVar(&user, "user", "", "按 username 过滤 (精确匹配)")
	cc.Flags().IntVar(&limit, "limit", 20, "限制数量")
	cc.Flags().BoolVar(&jsonOutput, "json", false, "输出 JSON")
	ht.Inject(cc)
	return cc
}

// --- show---

func showCmd() *cobra.Command {
	var latest bool
	var jsonOutput bool
	ht := MustParse("show")
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

			tags, _ := store.GetTags(r.ID)
			note, _ := store.GetNote(r.ID)
			if jsonOutput {
				payload := runJSON(r, tags, note)
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}

			detail := &cmd.RunDetail{
				ID:                r.ID,
				Name:              r.Name,
				Project:           r.Project,
				Status:            r.Status,
				DisplayStatus:     r.DisplayStatus(),
				Command:           r.Command,
				CWD:               r.CWD,
				StartedAt:         r.StartedAt,
				EndedAt:           r.EndedAt,
				Duration:          cmd.DisplayDuration(r.Status, r.StartedAt, r.DurationMs),
				ExitCode:          r.ExitCode,
				PeakRSSKB:         r.PeakRSSKB,
				CPUTimeMs:         r.CPUTimeMs,
				ResourceSupported: r.ResourceSupported,
				ResourceStatus:    r.ResourceStatus,
				GitRepo:           r.GitRepo,
				GitCommit:         r.GitCommit,
				GitDirty:          r.GitDirty,
				Tags:              tags,
				Note:              note,
				Diag:              toCmdDiagnosticDetail(internal.DiagnosticSummaryFromRun(r)),
				CWDSource:         r.CWDSource,
				ProjectSource:     r.ProjectSource,
			}
			fmt.Print(cmd.FormatShow(detail))
			return nil
		},
	}
	c.Flags().BoolVar(&latest, "latest", false, "查看最新运行")
	c.Flags().BoolVar(&jsonOutput, "json", false, "输出 JSON")
	ht.Inject(c)
	return c
}

type runJSONPayload struct {
	ID                string                     `json:"id"`
	Name              string                     `json:"name,omitempty"`
	Project           string                     `json:"project"`
	ProjectSource     string                     `json:"project_source,omitempty"`
	CWD               string                     `json:"cwd"`
	CWDSource         string                     `json:"cwd_source,omitempty"`
	Command           string                     `json:"command"`
	Status            string                     `json:"status"`
	DisplayStatus     string                     `json:"display_status,omitempty"`
	ExitCode          int                        `json:"exit_code"`
	StartedAt         string                     `json:"started_at"`
	EndedAt           string                     `json:"ended_at,omitempty"`
	DurationMs        int64                      `json:"duration_ms"`
	RunDir            string                     `json:"run_dir"`
	Hostname          string                     `json:"hostname,omitempty"`
	HostnameStatus    string                     `json:"hostname_status,omitempty"`
	Username          string                     `json:"username,omitempty"`
	UsernameStatus    string                     `json:"username_status,omitempty"`
	GitRepo           string                     `json:"git_repo,omitempty"`
	GitBranch         string                     `json:"git_branch,omitempty"`
	GitCommit         string                     `json:"git_commit,omitempty"`
	GitDirty          bool                       `json:"git_dirty"`
	CondaStatus       string                     `json:"conda_status,omitempty"`
	CondaEnv          string                     `json:"conda_env,omitempty"`
	CondaPrefix       string                     `json:"conda_prefix,omitempty"`
	PythonVersion     string                     `json:"python_version,omitempty"`
	ResourceSupported bool                       `json:"resource_supported"`
	ResourceStatus    string                     `json:"resource_status,omitempty"`
	PeakRSSKB         int64                      `json:"peak_rss_kb"`
	CPUTimeMs         int64                      `json:"cpu_time_ms"`
	Tags              []string                   `json:"tags,omitempty"`
	Note              string                     `json:"note,omitempty"`
	DiagnosticSummary internal.DiagnosticSummary `json:"diagnostic_summary"`
}

func runJSON(r *internal.Run, tags []string, note string) runJSONPayload {
	return runJSONPayload{
		ID:                r.ID,
		Name:              r.Name,
		Project:           r.Project,
		ProjectSource:     r.ProjectSource,
		CWD:               r.CWD,
		CWDSource:         r.CWDSource,
		Command:           r.Command,
		Status:            r.Status,
		DisplayStatus:     r.DisplayStatus(),
		ExitCode:          r.ExitCode,
		StartedAt:         r.StartedAt,
		EndedAt:           r.EndedAt,
		DurationMs:        r.DurationMs,
		RunDir:            r.RunDir,
		Hostname:          r.Hostname,
		HostnameStatus:    r.HostnameStatus,
		Username:          r.Username,
		UsernameStatus:    r.UsernameStatus,
		GitRepo:           r.GitRepo,
		GitBranch:         r.GitBranch,
		GitCommit:         r.GitCommit,
		GitDirty:          r.GitDirty,
		CondaStatus:       r.CondaStatus,
		CondaEnv:          r.CondaEnv,
		CondaPrefix:       r.CondaPrefix,
		PythonVersion:     r.PythonVersion,
		ResourceSupported: r.ResourceSupported,
		ResourceStatus:    r.ResourceStatus,
		PeakRSSKB:         r.PeakRSSKB,
		CPUTimeMs:         r.CPUTimeMs,
		Tags:              tags,
		Note:              note,
		DiagnosticSummary: internal.DiagnosticSummaryFromRun(r),
	}
}

func runSelectorArgs(latest *bool) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if latest != nil && *latest {
			if len(args) != 0 {
				return cliError("ambiguous_run_selector", "--latest 不能和 run_id 同时使用", "删除 run_id 参数，或删除 --latest 后指定一个真实 run_id", nil)
			}
			return nil
		}
		if len(args) != 1 {
			return cliError("missing_run_selector", "需要 run_id，或使用 --latest", "先运行 brun list --limit 5 查看 run_id，或使用 --latest", nil)
		}
		return nil
	}
}

func runLookupError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "no runs found") {
		return cliError("no_runs_found", msg, "先使用 brun run -- <command> 创建运行记录", err)
	}
	if strings.Contains(msg, "not found") {
		return cliError("run_not_found", msg, "运行 brun list --limit 20 查看可用 run_id，或使用 --latest", err)
	}
	return err
}

func selectedRun(store *internal.Store, args []string, latest bool) (*internal.Run, error) {
	if latest {
		return store.GetLatestRun()
	}
	return store.GetRun(args[0])
}

func diagnosticSummaryLabel(r *internal.Run) string {
	summary := internal.DiagnosticSummaryFromRun(r)
	if summary.ErrorCount > 0 {
		return fmt.Sprintf("E%d", summary.ErrorCount)
	}
	if summary.WarningCount > 0 {
		return fmt.Sprintf("W%d", summary.WarningCount)
	}
	return "-"
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
	var latest bool

	ht := MustParse("script")
	c := &cobra.Command{
		// Accept 1 or 2 args: single run view, or diff two runs
		Args: func(cc *cobra.Command, args []string) error {
			if latest && len(args) != 0 {
				return cliError("ambiguous_run_selector", "--latest 不能和 run_id 同时使用", "删除 run_id 参数，或删除 --latest 后指定一个真实 run_id", nil)
			}
			if !latest && (len(args) == 0 || len(args) > 2) {
				return cliError("missing_run_selector", "需要 1 个 run_id（查看脚本）或 2 个 run_id（对比差异），或使用 --latest", "先运行 brun list --limit 5 查看 run_id，或使用 --latest", nil)
			}
			if pathOnly && len(args) == 2 {
				return cliError("incompatible_flags", "--path 不能与双 run_id diff 模式同时使用", "删除 --path，或只指定一个 run_id", nil)
			}
			return nil
		},
		RunE: func(cc *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			// C: dual-id diff mode
			if len(args) == 2 {
				return scriptDiffRunE(cc, store, args[0], args[1])
			}

			// Single-run view (original logic + A/B/D enhancements)
			r, err := selectedRun(store, args, latest)
			if err != nil {
				return runLookupError(err)
			}

			snapshot, err := cmd.ReadScriptSnapshot(r.RunDir)
			if err != nil {
				return err
			}
			if pathOnly {
				fmt.Fprintln(cc.OutOrStdout(), snapshot.Path)
				return nil
			}

			out := cc.OutOrStdout()

			// B: print metadata header before content
			header := formatScriptHeader(r, snapshot)
			if header != "" {
				fmt.Fprintln(out, header)
			}

			// A: fallback warning line
			if snapshot.IsFallback {
				fmt.Fprintln(out, "# ⚠ 原始命令 (非脚本文件快照，来自 command.sh)")
			}

			content := snapshot.Content

			// D: auto-pager for long content
			if shouldPager(content) {
				pagerCmd := lookupPager()
				pagerCmd.Stdin = strings.NewReader(content)
				pagerCmd.Stdout = out
				pagerCmd.Stderr = os.Stderr
				if err := pagerCmd.Run(); err != nil {
					// pager failed, fall back to direct output
					fmt.Fprint(out, content)
				}
				return nil
			}

			fmt.Fprint(out, content)
			return nil
		},
	}
	c.Flags().BoolVar(&pathOnly, "path", false, "只输出脚本快照路径")
	c.Flags().BoolVar(&latest, "latest", false, "查看最新运行")
	ht.Inject(c)
	return c
}

// formatScriptHeader builds a metadata summary line shown before script content.
func formatScriptHeader(r *internal.Run, snap cmd.ScriptSnapshot) string {
	var b strings.Builder
	name := snap.Name
	if r.Name != "" {
		name = fmt.Sprintf("%s (run: %s)", name, r.ID)
	}
	sizeStr := fmt.Sprintf("%.1f KB", float64(len(snap.Content))/1024)

	fmt.Fprintf(&b, "# ── %s (%s) ──", name, sizeStr)

	// Status + duration
	duration := cmd.DisplayDuration(r.Status, r.StartedAt, r.DurationMs)
	statusLabel := r.Status
	if ds := r.DisplayStatus(); ds != "" && ds != r.Status {
		statusLabel = cmd.DisplayStatusLabel(r.Status, ds)
	}
	fmt.Fprintf(&b, " │ %s │ %s", cmd.StatusColor(statusLabel), duration)

	// Exit code
	fmt.Fprintf(&b, " │ Exit: %d", r.ExitCode)

	// CWD (truncated)
	cwd := r.CWD
	if len(cwd) > 40 {
		cwd = "..." + cwd[len(cwd)-37:]
	}
	fmt.Fprintf(&b, " │ %s", cmd.Dim(cwd))

	b.WriteString("\n")
	return b.String()
}

// shouldPager returns true if content has more lines than the terminal height.
// Returns false when stdout is not a TTY (pipe mode — no pager needed).
func shouldPager(content string) bool {
	h := getTerminalRows()
	if h == 0 { // not a TTY
		return false
	}
	lineCount := 0
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		lineCount++
		if lineCount > h-4 { // -4 for header/padding
			return true
		}
	}
	return false
}

// getTerminalRows returns terminal height (rows). Returns 0 when stdout is not a TTY.
func getTerminalRows() int {
	var ws struct{ Row, Col, Xpixel, Ypixel uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(os.Stdout.Fd()),
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno == 0 && ws.Row > 0 {
		return int(ws.Row)
	}
	if h := os.Getenv("LINES"); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > 0 {
			return n
		}
	}
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	if out, err := cmd.Output(); err == nil {
		parts := strings.Fields(string(out))
		if len(parts) == 2 {
			if n, err := strconv.Atoi(parts[0]); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0 // not detectable → treat as pipe, no pager
}

// lookupPager returns a *exec.Cmd configured to use $PAGER or less.
func lookupPager() *exec.Cmd {
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less -FRX"
	}
	parts := strings.Fields(pager)
	cmd := exec.Command(parts[0], parts[1:]...)
	return cmd
}

// scriptDiffRunE handles `brun script id1 id2` — unified diff of two script snapshots.
func scriptDiffRunE(cc *cobra.Command, store *internal.Store, id1, id2 string) error {
	r1, err := store.GetRun(id1)
	if err != nil {
		return runLookupError(err)
	}
	r2, err := store.GetRun(id2)
	if err != nil {
		return runLookupError(err)
	}

	snap1, err := cmd.ReadScriptSnapshot(r1.RunDir)
	if err != nil {
		return fmt.Errorf("读取 %s 脚本失败: %w", id1, err)
	}
	snap2, err := cmd.ReadScriptSnapshot(r2.RunDir)
	if err != nil {
		return fmt.Errorf("读取 %s 脚本失败: %w", id2, err)
	}

	// Write both contents to temp files for system diff
	tmp1, err := os.CreateTemp("", "brun-script-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp1.Name())
	tmp1.WriteString(snap1.Content)
	tmp1.Close()

	tmp2, err := os.CreateTemp("", "brun-script-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp2.Name())
	tmp2.WriteString(snap2.Content)
	tmp2.Close()

	labelA := fmt.Sprintf("a/%s (%s)", snap1.Name, id1[:8])
	labelB := fmt.Sprintf("b/%s (%s)", snap2.Name, id2[:8])

	diffCmd := exec.Command("diff", "-u", "--label", labelA, "--label", labelB, tmp1.Name(), tmp2.Name())
	diffCmd.Stdout = cc.OutOrStdout()
	diffCmd.Stderr = os.Stderr

	if err := diffCmd.Run(); err != nil {
		// diff returns exit code 1 when files differ — that's expected, not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("diff 执行失败: %w", err)
	}
	return nil
}

// --- logs---

func logsCmd() *cobra.Command {
	var stdoutOnly, stderrOnly bool
	var tailN int
	var follow, latest bool

	ht := MustParse("logs")
	c := &cobra.Command{
		Args: runSelectorArgs(&latest),
		RunE: func(c *cobra.Command, args []string) error {
			if stdoutOnly && stderrOnly {
				return cliError("incompatible_flags", "--stdout 和 --stderr 不能同时使用", "只保留其中一个选项", nil)
			}
			if tailN < 0 {
				return cliError("invalid_tail", "--tail 不能为负数", "使用 0 显示全部，或指定正整数", nil)
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			r, err := selectedRun(store, args, latest)
			if err != nil {
				return runLookupError(err)
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
	c.Flags().BoolVarP(&follow, "follow", "f", false, "持续跟踪 (类似 tail -f；默认跟踪 stdout)")
	c.Flags().BoolVar(&latest, "latest", false, "查看最新运行")
	ht.Inject(c)
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
	var latest bool
	var jsonOutput bool
	ht := MustParse("outputs")
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

			arts, err := store.GetArtifacts(r.ID)
			if err != nil {
				return err
			}

			if jsonOutput {
				payload := struct {
					RunID     string         `json:"run_id"`
					Project   string         `json:"project"`
					Artifacts []artifactJSON `json:"artifacts"`
				}{
					RunID:   r.ID,
					Project: r.Project,
				}
				payload.Artifacts = make([]artifactJSON, len(arts))
				for i, artifact := range arts {
					payload.Artifacts[i] = artifactJSONFromInternal(artifact)
				}
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
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
	c.Flags().BoolVar(&latest, "latest", false, "查看最新运行")
	c.Flags().BoolVar(&jsonOutput, "json", false, "输出 JSON")
	ht.Inject(c)
	return c
}

type artifactJSON struct {
	ID            int64  `json:"id"`
	RunID         string `json:"run_id"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	Path          string `json:"path"`
	AbsPath       string `json:"abs_path,omitempty"`
	StoredPath    string `json:"stored_path,omitempty"`
	Size          int64  `json:"size_bytes"`
	SHA256        string `json:"sha256,omitempty"`
	Mtime         string `json:"mtime,omitempty"`
	CaptureMethod string `json:"capture_method,omitempty"`
}

func artifactJSONFromInternal(a *internal.Artifact) artifactJSON {
	if a == nil {
		return artifactJSON{}
	}
	return artifactJSON{
		ID:            a.ID,
		RunID:         a.RunID,
		Kind:          a.Kind,
		Status:        a.Status,
		Path:          a.Path,
		AbsPath:       a.AbsPath,
		StoredPath:    a.StoredPath,
		Size:          a.Size,
		SHA256:        a.SHA256,
		Mtime:         a.Mtime,
		CaptureMethod: a.CaptureMethod,
	}
}

func diagCmd() *cobra.Command {
	var latest, all, jsonOutput bool
	ht := MustParse("diag")
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
			events, err := internal.ReadDiagnostics(r.RunDir)
			if err != nil {
				return err
			}
			summary := internal.SummarizeDiagnostics(events)
			visible := events
			if !all {
				visible = internal.DiagnosticWarnings(events)
			}
			if jsonOutput {
				payload := struct {
					RunID       string                     `json:"run_id"`
					Summary     internal.DiagnosticSummary `json:"summary"`
					Diagnostics []internal.DiagnosticEvent `json:"diagnostics"`
				}{
					RunID:       r.ID,
					Summary:     summary,
					Diagnostics: visible,
				}
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			fmt.Fprintf(c.OutOrStdout(), "Run ID: %s\n", r.ID)
			fmt.Fprintf(c.OutOrStdout(), "诊断: %d info, %d warning, %d error\n", summary.InfoCount, summary.WarningCount, summary.ErrorCount)
			if len(visible) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "未找到诊断事件。")
				return nil
			}
			for _, event := range visible {
				fmt.Fprintf(c.OutOrStdout(), "%s  %s  %s\n", event.Level, event.Code, event.Message)
				if event.Detail != "" {
					fmt.Fprintf(c.OutOrStdout(), "  %s\n", event.Detail)
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&latest, "latest", false, "查看最新运行")
	c.Flags().BoolVar(&all, "all", false, "显示 info/warning/error 全部诊断")
	c.Flags().BoolVar(&jsonOutput, "json", false, "输出 JSON")
	ht.Inject(c)
	return c
}

// --- tag---

func parseTimeFilter(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	// 尝试直接解析为日期
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return s, nil
	}

	// 相对时间
	now := time.Now().UTC()
	if s == "today" {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UTC().Format(time.RFC3339), nil
	}
	if len(s) < 2 {
		return "", invalidTimeFilterError(s)
	}
	unit := s[len(s)-1]
	if unit != 'h' && unit != 'd' && unit != 'w' {
		return "", invalidTimeFilterError(s)
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return "", invalidTimeFilterError(s)
	}
	switch unit {
	case 'h':
		return now.Add(-time.Duration(n) * time.Hour).Format(time.RFC3339), nil
	case 'd':
		return now.Add(-time.Duration(n) * 24 * time.Hour).Format(time.RFC3339), nil
	case 'w':
		return now.Add(-time.Duration(n) * 7 * 24 * time.Hour).Format(time.RFC3339), nil
	}
	return "", invalidTimeFilterError(s)
}

func invalidTimeFilterError(s string) error {
	return fmt.Errorf("无效时间 %q，支持 YYYY-MM-DD、RFC3339、today、Nh、Nd、Nw", s)
}

func parseListStatusFilter(status string) (string, bool, error) {
	if status == "" {
		return "", false, nil
	}
	withWarnings := strings.HasSuffix(status, "_with_warnings")
	base := strings.TrimSuffix(status, "_with_warnings")
	switch base {
	case "running", "success", "failed", "cancelled":
		if withWarnings && base == "running" {
			return "", false, cliError("invalid_status", "running 没有 with_warnings 变体", "使用 --status running", nil)
		}
		return base, withWarnings, nil
	default:
		return "", false, cliError("invalid_status", "未知状态: "+status,
			"支持 running、success、failed、cancelled 及 success/failed/cancelled_with_warnings", nil)
	}
}

// --- web ---
