package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biotools/brun/internal"
)

func TestGenerateInitYAML(t *testing.T) {
	yaml := GenerateInitYAML("my-project")
	if !strings.Contains(yaml, "project: my-project") {
		t.Errorf("yaml should contain project: my-project, got: %s", yaml)
	}
	if !strings.Contains(yaml, "capture:") {
		t.Error("yaml should contain capture section")
	}
	if !strings.Contains(yaml, "ignore:") {
		t.Error("yaml should contain ignore section")
	}
}

func TestWriteInitFile(t *testing.T) {
	tmp := t.TempDir()
	err := WriteInitFile(tmp, "test-proj")
	if err != nil {
		t.Fatalf("WriteInitFile() error = %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, "brun.yaml"))
	if !strings.Contains(string(data), "project: test-proj") {
		t.Errorf("brun.yaml content wrong: %s", string(data))
	}
}

func TestFormatCleanSummary(t *testing.T) {
	items := []CleanItem{
		{RunID: "r1", Age: "90d", Size: "100MB", Reason: "old"},
		{RunID: "r2", Age: "120d", Size: "200MB", Reason: "old"},
	}

	output := FormatCleanSummary(items, false)
	if !strings.Contains(output, "r1") {
		t.Error("summary should contain r1")
	}
	if strings.Contains(output, "Would remove") {
		t.Error("non-dry-run should not show Would remove")
	}
}

func TestFormatCleanSummary_DryRun(t *testing.T) {
	items := []CleanItem{{RunID: "r1", Age: "90d", Size: "100MB", Reason: "old"}}
	output := FormatCleanSummary(items, true)
	if !strings.Contains(output, "Would remove") {
		t.Error("dry-run should show Would remove")
	}
}

func TestBuildRerunCommand(t *testing.T) {
	run := &internal.Run{
		ID:      "20260513-153012-a8f3c2",
		CWD:     "/home/user/project",
		Command: "python script.py --sample S1 --output results/S1.txt",
	}

	cmd, cwd := BuildRerunCommand(run, "", false)
	if cmd != run.Command {
		t.Errorf("command = %q, want %q", cmd, run.Command)
	}
	if cwd != run.CWD {
		t.Errorf("cwd = %q, want %q", cwd, run.CWD)
	}
}

func TestBuildRerunCommand_WithNewCWD(t *testing.T) {
	run := &internal.Run{
		ID:      "test-001",
		CWD:     "/old/path",
		Command: "echo hello",
	}

	_, cwd := BuildRerunCommand(run, "/new/path", false)
	if cwd != "/new/path" {
		t.Errorf("cwd = %q, want /new/path", cwd)
	}
}

func TestBuildMetadataYAML(t *testing.T) {
	run := &internal.Run{
		ID:        "20260513-153012-a8f3c2",
		Name:      "test",
		Project:   "proj",
		Command:   "echo hi",
		Status:    "success",
		ExitCode:  0,
		RunDir:    "/tmp/run",
		GitCommit: "abc1234",
	}

	yaml := BuildMetadataYAML(run)
	checks := []string{"20260513-153012-a8f3c2", "test", "proj", "echo hi", "success", "abc1234"}
	for _, c := range checks {
		if !strings.Contains(yaml, c) {
			t.Errorf("metadata missing %q", c)
		}
	}
}
