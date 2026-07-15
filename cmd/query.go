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
	ID            string
	Name          string
	Project       string
	Status        string
	DisplayStatus string
	Diagnostic    string
	Duration      string
	Command       string
	CWD           string
}

type RunDetail struct {
	ID                   string
	Name                 string
	Project              string
	ProjectSource        string
	Status               string
	DisplayStatus        string
	Command              string
	CWD                  string
	CWDSource            string
	StartedAt            string
	EndedAt              string
	Duration             string
	ExitCode             int
	PeakRSSKB            int64
	CPUTimeMs            int64
	ResourceSupported    bool
	ResourceStatus       string
	ResourceRequested    string
	ResourceBackend      string
	ResourceCgroupPath   string
	ResourceFallback     string
	MemoryPeakBytes      int64
	CPUUserMs            int64
	CPUSystemMs          int64
	IOReadBytes          int64
	IOWriteBytes         int64
	OOMKillCount         int64
	PIDsPeak             int64
	GitRepo              string
	GitCommit            string
	GitDirty             bool
	Tags                 []string
	Note                 string
	Diag                 DiagnosticDetail
	ProcessPID           int
	ProcessPGID          int
	TerminationReason    string
	TerminationSignal    string
	TerminationEscalated bool
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
	return formatRunList(runs, getTermWidth())
}

const runListIDWidth = 24

func formatRunList(runs []RunRow, termWidth int) string {
	if len(runs) == 0 {
		return Gray("未找到运行记录。\n")
	}
	var b strings.Builder
	stacked := termWidth > 0 && termWidth < 78
	hasNames := false
	for _, r := range runs {
		if r.Name != "" {
			hasNames = true
			break
		}
	}
	wideMinWidth := 90
	if hasNames {
		wideMinWidth = 92
	}
	compact := termWidth > 0 && termWidth < wideMinWidth

	var headers []string
	var widths []int
	if stacked {
		headers = []string{"RUN ID", "STATUS", "DURATION"}
		widths = []int{runListIDWidth, 20, 10}
	} else if compact {
		headers = []string{"RUN ID", "PROJECT", "STATUS", "DIAG", "DURATION"}
		widths = []int{runListIDWidth, runListMax(14, termWidth-64), 20, 6, 10}
	} else {
		headers, widths = wideRunListColumns(termWidth, hasNames)
	}
	b.WriteString(Bold(formatTableRow(headers, widths)))
	b.WriteByte('\n')
	b.WriteString(Dim(formatTableDivider(widths)))
	b.WriteByte('\n')

	for _, r := range runs {
		statusLabel := DisplayStatusLabel(r.Status, r.DisplayStatus)
		var values []string
		if stacked {
			values = []string{RunIDColor(r.ID), StatusColor(statusLabel), DurationColor(r.Status, r.Duration)}
			b.WriteString(formatTableRow(values, widths))
			b.WriteByte('\n')
			fmt.Fprintf(&b, "  %s %s  %s %s\n",
				Dim("project:"), ProjectColor(r.Project), Dim("diag:"), DiagnosticColor(r.Diagnostic))
		} else if compact {
			values = []string{
				RunIDColor(r.ID), ProjectColor(r.Project), StatusColor(statusLabel),
				DiagnosticColor(r.Diagnostic), DurationColor(r.Status, r.Duration),
			}
			b.WriteString(formatTableRow(values, widths))
			b.WriteByte('\n')
		} else {
			values = []string{RunIDColor(r.ID)}
			if hasNames {
				values = append(values, NameColor(r.Name))
			}
			values = append(values,
				ProjectColor(r.Project), StatusColor(statusLabel),
				DiagnosticColor(r.Diagnostic), DurationColor(r.Status, r.Duration))
			b.WriteString(formatTableRow(values, widths))
			b.WriteByte('\n')
		}
		if compact && r.Name != "" {
			fmt.Fprintf(&b, "  %s %s\n", Dim("name:"), NameColor(r.Name))
		}

		writeRunListDetail(&b, "cmd", r.Command, termWidth)
		if r.CWD != "" {
			writeRunListDetail(&b, "cwd", r.CWD, termWidth)
		}
	}
	return b.String()
}

func wideRunListColumns(termWidth int, hasNames bool) ([]string, []int) {
	if !hasNames {
		projectWidth := 36
		if termWidth > 0 {
			projectWidth = runListMin(36, runListMax(14, termWidth-64))
		}
		return []string{"RUN ID", "PROJECT", "STATUS", "DIAG", "DURATION"},
			[]int{runListIDWidth, projectWidth, 20, 6, 10}
	}

	nameWidth, projectWidth := 24, 36
	if termWidth > 0 {
		extra := runListMax(0, termWidth-91)
		nameExtra := runListMin(12, extra/2)
		projectExtra := runListMin(22, extra-nameExtra)
		nameWidth = 12 + nameExtra
		projectWidth = 14 + projectExtra
	}
	return []string{"RUN ID", "NAME", "PROJECT", "STATUS", "DIAG", "DURATION"},
		[]int{runListIDWidth, nameWidth, projectWidth, 20, 6, 10}
}

func formatTableRow(values []string, widths []int) string {
	cells := make([]string, len(widths))
	for i, width := range widths {
		if i < len(values) {
			cells[i] = PadRight(values[i], width)
		} else {
			cells[i] = strings.Repeat(" ", width)
		}
	}
	return strings.Join(cells, " ")
}

func formatTableDivider(widths []int) string {
	parts := make([]string, len(widths))
	for i, width := range widths {
		parts[i] = strings.Repeat("-", width)
	}
	return strings.Join(parts, " ")
}

func runListMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func runListMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func DisplayStatusLabel(status, displayStatus string) string {
	if displayStatus == "" || displayStatus == status {
		return status
	}
	suffix := strings.TrimPrefix(displayStatus, status+"_")
	suffix = strings.TrimPrefix(suffix, "with_")
	return status + "(+" + suffix + ")"
}

func writeRunListDetail(b *strings.Builder, label, value string, termWidth int) {
	prefix := "  " + label + ": "
	lineWidth := 0
	if termWidth > 0 {
		lineWidth = termWidth - visibleWidth(prefix)
	}
	lines := wrapDisplayText(value, lineWidth)
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(b, "  %s%s %s\n", DetailLabelColor(label), Dim(":"), Dim(line))
		} else {
			fmt.Fprintf(b, "%s%s\n", strings.Repeat(" ", visibleWidth(prefix)), Dim(line))
		}
	}
}

// wrapDisplayText wraps plain text to terminal cells. It prefers whitespace
// boundaries and only hard-wraps when one token is wider than the line.
func wrapDisplayText(text string, width int) []string {
	if width <= 0 || visibleWidth(text) <= width {
		return []string{text}
	}
	var lines []string
	remaining := text
	for remaining != "" {
		if visibleWidth(remaining) <= width {
			lines = append(lines, remaining)
			break
		}
		head, tail := splitDisplayWidth(remaining, width)
		if breakAt := strings.LastIndexByte(head, ' '); breakAt > len(head)/2 {
			tail = remaining[breakAt+1:]
			head = head[:breakAt]
		} else if breakAt := strings.LastIndexByte(head, '/'); breakAt > len(head)/2 {
			tail = remaining[breakAt+1:]
			head = head[:breakAt+1]
		}
		lines = append(lines, strings.TrimSpace(head))
		remaining = strings.TrimSpace(tail)
	}
	return lines
}

func splitDisplayWidth(s string, width int) (string, string) {
	used := 0
	for i, r := range s {
		runeWidth := visibleWidth(string(r))
		if used+runeWidth > width {
			if i == 0 {
				return string(r), s[len(string(r)):]
			}
			return s[:i], s[i:]
		}
		used += runeWidth
	}
	return s, ""
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
	if r.ProcessPID > 0 {
		fmt.Fprintf(&b, "%s  PID %d / PGID %d\n", Bold("Process:"), r.ProcessPID, r.ProcessPGID)
	}
	if r.TerminationReason != "" {
		fmt.Fprintf(&b, "%s  %s", Bold("Termination:"), r.TerminationReason)
		if r.TerminationSignal != "" {
			fmt.Fprintf(&b, " (%s)", r.TerminationSignal)
		}
		if r.TerminationEscalated {
			b.WriteString(" escalated")
		}
		b.WriteString("\n")
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
	if r.ResourceBackend != "" {
		fmt.Fprintf(&b, "%s  %s", Bold("Resource Backend:"), r.ResourceBackend)
		if r.ResourceRequested != "" && r.ResourceRequested != r.ResourceBackend {
			fmt.Fprintf(&b, " (requested %s)", r.ResourceRequested)
		}
		b.WriteString("\n")
	}
	if r.ResourceFallback != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Resource Fallback:"), r.ResourceFallback)
	}
	if r.MemoryPeakBytes > 0 {
		fmt.Fprintf(&b, "%s  %s (cgroup charged)\n", Bold("Memory Peak:"), FormatSize(r.MemoryPeakBytes))
	}
	if r.CPUUserMs > 0 || r.CPUSystemMs > 0 {
		fmt.Fprintf(&b, "%s  user %s / system %s\n", Bold("CPU Breakdown:"), fmtCPU(r.CPUUserMs), fmtCPU(r.CPUSystemMs))
	}
	if r.IOReadBytes > 0 || r.IOWriteBytes > 0 {
		fmt.Fprintf(&b, "%s  read %s / write %s\n", Bold("I/O:"), FormatSize(r.IOReadBytes), FormatSize(r.IOWriteBytes))
	}
	if r.PIDsPeak > 0 || r.OOMKillCount > 0 {
		fmt.Fprintf(&b, "%s  pids peak %d / oom kills %d\n", Bold("Resource Events:"), r.PIDsPeak, r.OOMKillCount)
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
	if (status == "running" || status == "starting") && startedAt != "" {
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
