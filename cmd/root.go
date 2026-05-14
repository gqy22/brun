package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/biotools/brun/internal"
)

type CleanItem struct {
	RunID  string
	Age    string
	Size   string
	Reason string
}

func GenerateInitYAML(project string) string {
	return fmt.Sprintf(`project: %s

capture:
  scripts:
    - "scripts/**/*.py"
    - "scripts/**/*.R"
    - "*.sh"
    - "Snakefile"
    - "*.nf"
  configs:
    - "configs/**/*.yaml"
    - "configs/**/*.yml"
    - "samples.tsv"
  outputs:
    - "results/**/*"
    - "reports/**/*"

ignore:
  - ".git/**"
  - ".snakemake/**"
  - ".nextflow/**"
  - "work/**"
  - "tmp/**"
  - "__pycache__/**"
  - "*.tmp"
  - "*.swp"
`, project)
}

func WriteInitFile(dir, project string) error {
	yaml := GenerateInitYAML(project)
	return os.WriteFile(filepath.Join(dir, "brun.yaml"), []byte(yaml), 0644)
}

func FormatCleanSummary(items []CleanItem, dryRun bool) string {
	var b strings.Builder
	if len(items) == 0 {
		return "无需清理。\n"
	}
	prefix := "Will remove"
	if dryRun {
		prefix = "Would remove"
	}
	fmt.Fprintf(&b, "%s %d runs:\n", prefix, len(items))
	for _, item := range items {
		fmt.Fprintf(&b, "  %s  (%s, %s) - %s\n", item.RunID, item.Age, item.Size, item.Reason)
	}
	return b.String()
}

func BuildRerunCommand(run *internal.Run, newCWD string, inheritTags bool) (string, string) {
	cwd := run.CWD
	if newCWD != "" {
		cwd = newCWD
	}
	return run.Command, cwd
}

func BuildMetadataYAML(run *internal.Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "id: %s\n", run.ID)
	if run.Name != "" {
		fmt.Fprintf(&b, "name: %s\n", run.Name)
	}
	fmt.Fprintf(&b, "project: %s\n", run.Project)
	fmt.Fprintf(&b, "command: %s\n", run.Command)
	fmt.Fprintf(&b, "status: %s\n", run.Status)
	fmt.Fprintf(&b, "exit_code: %d\n", run.ExitCode)
	fmt.Fprintf(&b, "cwd: %s\n", run.CWD)
	if run.StartedAt != "" {
		fmt.Fprintf(&b, "started_at: %s\n", run.StartedAt)
	}
	if run.EndedAt != "" {
		fmt.Fprintf(&b, "ended_at: %s\n", run.EndedAt)
	}
	if run.DurationMs > 0 {
		fmt.Fprintf(&b, "duration_ms: %d\n", run.DurationMs)
	}
	if run.GitCommit != "" {
		fmt.Fprintf(&b, "git_commit: %s\n", run.GitCommit)
	}
	if run.GitDirty {
		b.WriteString("git_dirty: true\n")
	}
	return b.String()
}
