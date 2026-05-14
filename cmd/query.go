package cmd

import (
	"fmt"
	"strings"
)

type RunRow struct {
	ID       string
	Name     string
	Project  string
	Status   string
	Duration string
	Command  string
}

type RunDetail struct {
	ID        string
	Name      string
	Project   string
	Status    string
	Command   string
	CWD       string
	StartedAt string
	EndedAt   string
	Duration  string
	ExitCode  int
	GitRepo   string
	GitCommit string
	GitDirty  bool
	Tags      []string
	Note      string
}

type ArtifactRow struct {
	Kind   string
	Status string
	Size   string
	Path   string
}

func FormatRunList(runs []RunRow) string {
	if len(runs) == 0 {
		return Gray("未找到运行记录。\n")
	}
	var b strings.Builder
	b.WriteString(TableHeader("%-24s %-16s %-15s %-9s %-10s %s\n",
		"RUN ID", "NAME", "PROJECT", "STATUS", "DURATION", "COMMAND"))
	b.WriteString(Dim("----                     ----            -------         ------    --------   -------\n"))
	for _, r := range runs {
		name := r.Name
		if len(name) > 12 {
			name = name[:9] + "..."
		}
		cmd := r.Command
		if len(cmd) > 32 {
			cmd = cmd[:29] + "..."
		}
		fmt.Fprintf(&b, "%s %s %s %s %s %s\n",
			PadRight(r.ID, 24),
			PadRight(name, 16),
			PadRight(r.Project, 15),
			PadRight(StatusColor(r.Status), 9),
			PadRight(r.Duration, 10),
			Dim(cmd))
	}
	return b.String()
}

func FormatShow(r *RunDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", Bold("Run ID:"), r.ID)
	if r.Name != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Name:"), r.Name)
	}
	fmt.Fprintf(&b, "%s  %s\n", Bold("Project:"), r.Project)
	fmt.Fprintf(&b, "%s  %s\n", Bold("Status:"), StatusColor(r.Status))
	fmt.Fprintf(&b, "%s  %s\n", Bold("Command:"), r.Command)
	fmt.Fprintf(&b, "%s  %s\n", Bold("CWD:"), Dim(r.CWD))
	if r.StartedAt != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Started:"), r.StartedAt)
	}
	if r.EndedAt != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Ended:"), r.EndedAt)
	}
	if r.Duration != "" {
		fmt.Fprintf(&b, "%s  %s\n", Bold("Duration:"), r.Duration)
	}
	fmt.Fprintf(&b, "%s  %d\n", Bold("Exit Code:"), r.ExitCode)
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

func ResolveRunID(id string) (string, bool) {
	if id == "latest" || id == "" {
		return "", true
	}
	return id, false
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
