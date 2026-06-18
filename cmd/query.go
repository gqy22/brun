package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type RunRow struct {
	ID           string
	Name         string
	Project      string
	Status       string
	DisplayStatus string
	Diagnostic   string
	Duration     string
	Command      string
}

type RunDetail struct {
	ID                string
	Name              string
	Project           string
	ProjectSource     string
	Status            string
	DisplayStatus     string
	Command           string
	CWD               string
	CWDSource         string
	StartedAt         string
	EndedAt           string
	Duration          string
	ExitCode          int
	PeakRSSKB         int64
	CPUTimeMs         int64
	ResourceSupported bool
	ResourceStatus    string
	GitRepo           string
	GitCommit         string
	GitDirty          bool
	Tags              []string
	Note              string
	Diag              DiagnosticDetail
}

type DiagnosticDetail struct {
	InfoCount    int
	WarningCount int
	ErrorCount   int
	LastLevel    string
	LastCode     string
	LastMessage  string
	LastAt       string
}

type ArtifactRow struct {
	Kind   string
	Status string
	Size   string
	Path   string
}

type ScriptSnapshot struct {
	Name       string
	Path       string
	Content    string
	IsFallback bool // true when content comes from command.sh (raw command, not a script snapshot)
}

// ReadScriptSnapshot reads the saved input script snapshot from a run directory.
// It first looks for script.* files (saved when the command invoked a script file like bash align.sh).
// If none is found, it falls back to reading command.sh which always exists and contains
// the original command string. Binary files and files larger than 2MB are ignored.
func ReadScriptSnapshot(runDir string) (ScriptSnapshot, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return ScriptSnapshot{}, fmt.Errorf("读取 run 目录失败: %w", err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "script.") {
			continue
		}
		path := filepath.Join(runDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return ScriptSnapshot{}, fmt.Errorf("读取脚本快照失败: %w", err)
		}
		if len(data) > 2*1024*1024 || bytes.Contains(data, []byte{0}) {
			continue
		}
		return ScriptSnapshot{
			Name:    strings.TrimPrefix(e.Name(), "script."),
			Path:    path,
			Content: string(data),
		}, nil
	}

	// Fallback: no script.* found, read command.sh (always present, contains the raw command string)
	cmdPath := filepath.Join(runDir, "command.sh")
	data, err := os.ReadFile(cmdPath)
	if err == nil && len(data) > 0 && !bytes.Contains(data, []byte{0}) {
		return ScriptSnapshot{
			Name:       "command.sh",
			Path:       cmdPath,
			Content:    string(data),
			IsFallback: true,
		}, nil
	}

	return ScriptSnapshot{}, fmt.Errorf("未找到该 run 的脚本快照")
}

func FormatRunList(runs []RunRow) string {
	if len(runs) == 0 {
		return Gray("未找到运行记录。\n")
	}
	var b strings.Builder
	b.WriteString(TableHeader("%-24s %-16s %-15s %-22s %-6s %-10s\n",
		"RUN ID", "NAME", "PROJECT", "STATUS", "DIAG", "DURATION"))
	b.WriteString(Dim("----                     ----            -------         ------                ----   --------\n"))
	for _, r := range runs {
		name := r.Name
		if len(name) > 12 {
			name = name[:9] + "..."
		}
		statusLabel := r.Status
		if r.DisplayStatus != "" && r.DisplayStatus != r.Status {
			statusLabel = r.Status + "(+" + strings.TrimPrefix(r.DisplayStatus, r.Status+"_") + ")"
		}
		fmt.Fprintf(&b, "%s %s %s %s %s %s\n",
			PadRight(r.ID, 24),
			PadRight(name, 16),
			PadRight(r.Project, 15),
			PadRight(StatusColor(statusLabel), 22),
			PadRight(r.Diagnostic, 6),
			PadRight(r.Duration, 10))

		// Command as hanging block below the row
		wrapped := wrapCommand(r.Command)
		for i, line := range wrapped {
			if i == 0 {
				fmt.Fprintf(&b, "  %s\n", Dim(line))
			} else {
				fmt.Fprintf(&b, "    %s\n", Dim(line))
			}
		}
	}
	return b.String()
}

// wrapCommand splits a command into lines fitting within terminal width.
// Prefers breaking at spaces and --flag boundaries; falls back to hard break.
// When stdout is not a TTY (width=0), returns the command unwrapped.
func wrapCommand(cmd string) []string {
	width := getTermWidth()
	if width == 0 || len(cmd) <= width-2 {
		return []string{cmd}
	}

	firstLineWidth := width - 2
	contLineWidth := width - 4

	var lines []string
	remaining := cmd

	for len(remaining) > 0 {
		lineWidth := firstLineWidth
		if len(lines) > 0 {
			lineWidth = contLineWidth
		}

		if len(remaining) <= lineWidth {
			lines = append(lines, remaining)
			break
		}

		breakIdx := findBreakPoint(remaining[:lineWidth+1], lineWidth)
		lines = append(lines, remaining[:breakIdx])
		remaining = strings.TrimSpace(remaining[breakIdx:])
	}

	return lines
}

// findBreakPoint finds best position to break within limit chars.
// Priority: space before "--flag" > any space > hard cut at limit.
func findBreakPoint(s string, limit int) int {
	if len(s) <= limit {
		return len(s)
	}

	// Prefer breaking before a --flag (must have space or string-start before --)
	for i := limit - 1; i >= 2; i-- {
		if s[i] == '-' && s[i-1] == '-' && (i == 2 || s[i-2] == ' ') {
			return i - 1
		}
	}

	lastSpace := strings.LastIndex(s[:limit], " ")
	if lastSpace > limit/2 {
		return lastSpace
	}

	return limit
}

// getTermWidth returns terminal columns for stdout.
// Returns 0 when stdout is not a TTY (pipe/redirect) — callers should treat this as "unlimited".
func getTermWidth() int {
	// 1) Primary: ioctl on stdout fd — works for real terminals, fails cleanly for pipes
	var ws struct{ Row, Col, Xpixel, Ypixel uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(os.Stdout.Fd()),
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno == 0 && ws.Col > 0 {
		return int(ws.Col)
	}

	// 2) $COLUMNS — set by most shells, updated on resize
	if w := os.Getenv("COLUMNS"); w != "" {
		if n, err := strconv.Atoi(w); err == nil && n > 0 {
			return n
		}
	}

	// 3) stty size on stdin — fallback for edge cases
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	if out, err := cmd.Output(); err == nil {
		parts := strings.Fields(string(out))
		if len(parts) == 2 {
			if n, err := strconv.Atoi(parts[1]); err == nil && n > 0 {
				return n
			}
		}
	}

	// 4) Pipe / redirect mode: return 0 = unlimited (no wrapping)
	return 0
}

func FormatShow(r *RunDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", Bold("Run ID:"), r.ID)
	if r.Name != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Name:"), r.Name)
	}
	fmt.Fprintf(&b, "%s  %s\n", Bold("Project:"), r.Project)
	if r.ProjectSource != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Project Source:"), r.ProjectSource)
	}
	fmt.Fprintf(&b, "%s  %s\n", Bold("Status:"), StatusColor(r.Status))
	if r.DisplayStatus != "" && r.DisplayStatus != r.Status {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Display Status:"), StatusColor(r.DisplayStatus))
	}
	fmt.Fprintf(&b, "%s  %s\n", Bold("Command:"), r.Command)
	fmt.Fprintf(&b, "%s  %s\n", Bold("CWD:"), Dim(r.CWD))
	if r.CWDSource != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("CWD Source:"), r.CWDSource)
	}
	if r.StartedAt != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Started:"), r.StartedAt)
	}
	if r.EndedAt != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Ended:"), r.EndedAt)
	}
	if r.Duration != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Duration:"), r.Duration)
	}
	if r.PeakRSSKB > 0 || r.CPUTimeMs > 0 {
		if r.PeakRSSKB > 0 {
			fmt.Fprintf(&b, "%s  %s\n", Bold("Memory:"), fmtMem(r.PeakRSSKB))
		}
		if r.CPUTimeMs > 0 {
			fmt.Fprintf(&b, "%s  %s\n", Bold("CPU Time:"), fmtCPU(r.CPUTimeMs))
		}
	}
	if r.ResourceStatus != "" {
		fmt.Fprintf(&b, "%s  %s", Bold("Resource Status:"), r.ResourceStatus)
		if !r.ResourceSupported {
			fmt.Fprintf(&b, " (unsupported)")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s  %d\n", Bold("Exit Code:"), r.ExitCode)
	if r.Diag.WarningCount > 0 || r.Diag.ErrorCount > 0 {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Diagnostics:"), formatDiagCounts(r.Diag))
		if r.Diag.LastMessage != "" {
			fmt.Fprintf(&b, "%s  %s", Bold("Last Diagnostic:"), r.Diag.LastMessage)
			if r.Diag.LastCode != "" {
				fmt.Fprintf(&b, " (%s)", r.Diag.LastCode)
			}
			b.WriteString("\n")
		}
	}
	if r.GitRepo != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Git Repo:"), Dim(r.GitRepo))
	}
	if r.GitCommit != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Git Commit:"), Dim(r.GitCommit[:min(8, len(r.GitCommit))]))
	}
	if len(r.Tags) > 0 {
		tags := make([]string, len(r.Tags))
		for i, t := range r.Tags {
			tags[i] = Cyan(t)
		}
		fmt.Fprintf(&b, "%s  %s\n", Bold("Tags:"), strings.Join(tags, ", "))
	}
	if r.Note != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Note:"), r.Note)
	}
	return b.String()
}

func formatDiagCounts(d DiagnosticDetail) string {
	parts := []string{}
	if d.ErrorCount > 0 {
		parts = append(parts, fmt.Sprintf("%d error", d.ErrorCount))
	}
	if d.WarningCount > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", d.WarningCount))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func FormatOutputs(arts []ArtifactRow, runID, project string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", Bold("Run ID:"), runID)
	fmt.Fprintf(&b, "%s  %s\n\n", Bold("Project:"), project)
	if len(arts) == 0 {
		b.WriteString(Gray("未找到输出文件。\n"))
		return b.String()
	}
	b.WriteString(TableHeader("%-8s %-10s %-10s %s\n",
		"KIND", "STATUS", "SIZE", "PATH"))
	b.WriteString(Dim("----      ------     ----       ----\n"))
	for _, a := range arts {
		fmt.Fprintf(&b, "%-8s %-10s %-10s %s\n",
			KindColor(a.Kind),
			StatusColor(a.Status),
			a.Size,
			a.Path)
	}
	return b.String()
}

func FormatSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	case bytes < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	}
}

func TailLog(content string, n int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if n >= len(lines) {
		return content
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func fmtMem(kb int64) string {
	switch {
	case kb < 1024:
		return fmt.Sprintf("%d KB", kb)
	default:
		return fmt.Sprintf("%.1f MB", float64(kb)/1024)
	}
}

func fmtCPU(ms int64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	default:
		return fmt.Sprintf("%.2fs", float64(ms)/1000)
	}
}

func DisplayDuration(status, startedAt string, durationMs int64) string {
	if status == "running" && startedAt != "" {
		if started, err := time.Parse(time.RFC3339, startedAt); err == nil {
			ms := time.Since(started).Milliseconds()
			if ms < 0 {
				ms = 0
			}
			return DurationString(ms)
		}
	}
	return DurationString(durationMs)
}

func DurationString(ms int64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 60000:
		return fmt.Sprintf("%ds", ms/1000)
	case ms < 3600000:
		return fmt.Sprintf("%dm%ds", (ms/1000)/60, (ms/1000)%60)
	default:
		h := ms / 3600000
		m := (ms % 3600000) / 60000
		s := (ms % 60000) / 1000
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
}
