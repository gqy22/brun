package cmd

import (
	"fmt"
	"strings"
)

type RunRow struct {
	ID       string
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
		return "No runs found.\n"
	}
	var b strings.Builder
	b.WriteString("RUN ID                   PROJECT        STATUS    DURATION   COMMAND\n")
	b.WriteString("----                     -------         ------    --------   -------\n")
	for _, r := range runs {
		cmd := r.Command
		if len(cmd) > 40 {
			cmd = cmd[:37] + "..."
		}
		fmt.Fprintf(&b, "%-24s %-15s %-9s %-10s %s\n", r.ID, r.Project, r.Status, r.Duration, cmd)
	}
	return b.String()
}

func FormatShow(r *RunDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Run ID:     %s\n", r.ID)
	if r.Name != "" {
		fmt.Fprintf(&b, "Name:       %s\n", r.Name)
	}
	fmt.Fprintf(&b, "Project:    %s\n", r.Project)
	fmt.Fprintf(&b, "Status:     %s\n", r.Status)
	fmt.Fprintf(&b, "Command:    %s\n", r.Command)
	fmt.Fprintf(&b, "CWD:        %s\n", r.CWD)
	if r.StartedAt != "" {
		fmt.Fprintf(&b, "Started:    %s\n", r.StartedAt)
	}
	if r.EndedAt != "" {
		fmt.Fprintf(&b, "Ended:      %s\n", r.EndedAt)
	}
	if r.Duration != "" {
		fmt.Fprintf(&b, "Duration:   %s\n", r.Duration)
	}
	fmt.Fprintf(&b, "Exit Code:  %d\n", r.ExitCode)
	if r.GitRepo != "" {
		fmt.Fprintf(&b, "Git Repo:   %s\n", r.GitRepo)
	}
	if r.GitCommit != "" {
		fmt.Fprintf(&b, "Git Commit: %s\n", r.GitCommit)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(&b, "Tags:       %s\n", strings.Join(r.Tags, ", "))
	}
	if r.Note != "" {
		fmt.Fprintf(&b, "Note:       %s\n", r.Note)
	}
	return b.String()
}

func FormatOutputs(arts []ArtifactRow, runID, project string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Run ID: %s\n", runID)
	fmt.Fprintf(&b, "Project: %s\n\n", project)
	if len(arts) == 0 {
		b.WriteString("No outputs found.\n")
		return b.String()
	}
	b.WriteString("KIND      STATUS     SIZE       PATH\n")
	b.WriteString("----      ------     ----       ----\n")
	for _, a := range arts {
		fmt.Fprintf(&b, "%-8s %-10s %-10s %s\n", a.Kind, a.Status, a.Size, a.Path)
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
	lines := strings.Split(content, "\n")
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
