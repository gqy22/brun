package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatRunList(t *testing.T) {
	runs := []RunRow{
		{ID: "r1", Name: "test-1", Project: "p1", Status: "success", Duration: "1m23s", Command: "echo hi", CWD: "/work/project-a"},
		{ID: "r2", Name: "test-2", Project: "p2", Status: "failed", Duration: "0s", Command: "ls /x"},
	}

	output := FormatRunList(runs)
	if !strings.Contains(output, "r1") {
		t.Errorf("output should contain r1, got: %s", output)
	}
	if !strings.Contains(output, "success") {
		t.Errorf("output should contain success")
	}
	if !strings.Contains(output, "p2") {
		t.Errorf("output should contain p2")
	}
	if !strings.Contains(output, "cwd: /work/project-a") {
		t.Errorf("output should contain run cwd, got: %s", output)
	}
}

func TestFormatRunList_Empty(t *testing.T) {
	output := FormatRunList(nil)
	if output == "" {
		t.Error("empty list should still return header or message")
	}
}

func TestFormatRunList_ANSIColumnsAlign(t *testing.T) {
	oldUseColor := useColor
	useColor = true
	defer func() { useColor = oldUseColor }()

	runs := []RunRow{{
		ID: "20260714-120000-abcdef", Name: "test", Project: "project",
		Status: "success", DisplayStatus: "success_with_warnings",
		Diagnostic: "W=1", Duration: "1m2s", Command: "echo hi",
	}}
	lines := strings.Split(formatRunList(runs, 120), "\n")
	header := stripANSI(lines[0])
	row := stripANSI(lines[2])
	if got, want := strings.Index(row, "success"), strings.Index(header, "STATUS"); got != want {
		t.Fatalf("status starts at column %d, header at %d\nheader: %q\nrow:    %q", got, want, header, row)
	}
	if !strings.Contains(lines[2], "\033[32msuccess\033[0m\033[33m(+warnings)\033[0m") {
		t.Fatalf("warning status color sequence was truncated: %q", lines[2])
	}
}

func TestFormatRunList_LongRunIDCannotShiftColumns(t *testing.T) {
	runs := []RunRow{{
		ID: "custom-run-id-that-is-far-longer-than-the-column", Name: "named",
		Project: "unique-project", Status: "success", Diagnostic: "-",
		Duration: "1s", Command: "true",
	}}
	lines := strings.Split(formatRunList(runs, 120), "\n")
	header, row := stripANSI(lines[0]), stripANSI(lines[2])
	if !strings.Contains(row[:runListIDWidth], "...") {
		t.Fatalf("long run ID was not truncated inside its column: %q", row)
	}
	if got, want := strings.Index(row, "unique-project"), strings.Index(header, "PROJECT"); got != want {
		t.Fatalf("project starts at column %d, header at %d\nheader: %q\nrow:    %q", got, want, header, row)
	}
	if got, want := strings.Index(row, "success"), strings.Index(header, "STATUS"); got != want {
		t.Fatalf("status starts at column %d, header at %d\nheader: %q\nrow:    %q", got, want, header, row)
	}
}

func TestFormatRunList_WideLayoutHidesUnusedNameColumn(t *testing.T) {
	runs := []RunRow{{
		ID: "20260714-120000-abcdef", Project: "project", Status: "success",
		Diagnostic: "-", Duration: "1s", Command: "true",
	}}
	output := formatRunList(runs, 120)
	header := strings.Split(stripANSI(output), "\n")[0]
	if strings.Contains(header, "NAME") {
		t.Fatalf("wide header should hide NAME when every name is empty: %q", header)
	}
	if got, want := strings.Index(header, "PROJECT"), runListIDWidth+1; got != want {
		t.Fatalf("PROJECT starts at column %d, want %d immediately after RUN ID", got, want)
	}
}

func TestFormatRunList_CompactAt80Columns(t *testing.T) {
	runs := []RunRow{{
		ID: "20260714-120000-abcdef", Name: "样本分析", Project: "genome",
		Status: "running", Diagnostic: "-", Duration: "12m", Command: "echo hi",
	}}
	output := formatRunList(runs, 80)
	lines := strings.Split(output, "\n")
	if strings.Contains(lines[0], "NAME") {
		t.Fatalf("compact header should move NAME below the row: %q", lines[0])
	}
	if !strings.Contains(output, "name: 样本分析") {
		t.Fatalf("compact output missing name detail: %q", output)
	}
	if width := visibleWidth(lines[2]); width > 80 {
		t.Fatalf("compact data row width = %d, want <= 80: %q", width, lines[2])
	}
}

func TestFormatRunList_StackedAtNarrowWidth(t *testing.T) {
	runs := []RunRow{{
		ID: "r1", Project: "project", Status: "failed", Diagnostic: "E=1",
		Duration: "2s", Command: "false",
		CWD: "/home/user/a/very/long/project/directory/that/must/wrap/cleanly",
	}}
	output := formatRunList(runs, 60)
	if !strings.Contains(output, "project: project  diag: E=1") {
		t.Fatalf("stacked output missing project/diagnostic detail: %q", output)
	}
	for _, line := range strings.Split(output, "\n") {
		if width := visibleWidth(line); width > 60 {
			t.Fatalf("stacked line width = %d, want <= 60: %q", width, line)
		}
	}
}

func TestFormatShowOutput(t *testing.T) {
	run := &RunDetail{
		ID:        "20260513-153012-a8f3c2",
		Name:      "test-run",
		Project:   "rnaseq",
		Status:    "success",
		Command:   "python script.py --sample S1",
		CWD:       "/home/user/project",
		Duration:  "2m31s",
		ExitCode:  0,
		PeakRSSKB: 102400,
		CPUTimeMs: 15000,
		Tags:      []string{"rnaseq"},
		Note:      "test note",
	}

	output := FormatShow(run)
	checks := []string{"20260513-153012-a8f3c2", "test-run", "rnaseq", "success", "python script.py", "rnaseq", "test note", "100.0 MB", "15.00s"}
	for _, c := range checks {
		if !strings.Contains(output, c) {
			t.Errorf("show output missing %q", c)
		}
	}
}

func TestFormatShowOutput_NoResources(t *testing.T) {
	run := &RunDetail{
		ID: "r1", Name: "old", Project: "p", Status: "success",
		Command: "echo hi", ExitCode: 0,
	}

	output := FormatShow(run)
	if strings.Contains(output, "Memory:") {
		t.Error("should not show Memory when value is 0")
	}
	if strings.Contains(output, "CPU Time:") {
		t.Error("should not show CPU Time when value is 0")
	}
}

func TestFormatOutputs(t *testing.T) {
	arts := []ArtifactRow{
		{Kind: "output", Status: "created", Size: "8.4 GB", Path: "results/S1.bam"},
		{Kind: "output", Status: "created", Size: "3.2 MB", Path: "results/S1.bam.bai"},
		{Kind: "report", Status: "created", Size: "1.1 MB", Path: "reports/S1.html"},
	}

	output := FormatOutputs(arts, "test-run", "proj")
	if !strings.Contains(output, "S1.bam") {
		t.Error("outputs should contain S1.bam")
	}
	if !strings.Contains(output, "8.4 GB") {
		t.Error("outputs should contain size")
	}
}

func TestFormatSizeBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{8_400_000_000, "7.8 GB"},
	}
	for _, tt := range tests {
		got := FormatSize(tt.bytes)
		if got != tt.expected {
			t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.expected)
		}
	}
}

func TestTailLog(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\n"
	result := TailLog(content, 3)
	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Errorf("tail 3 lines, got %d lines: %q", len(lines), result)
	}
}

func TestDurationString(t *testing.T) {
	tests := []struct {
		ms       int64
		contains string
	}{
		{1000, "1s"},
		{60000, "1m"},
		{3600000, "1h"},
		{123456, "2m3s"},
	}
	for _, tt := range tests {
		got := DurationString(tt.ms)
		if !strings.Contains(got, tt.contains) {
			t.Errorf("DurationString(%d) = %q, want contain %q", tt.ms, got, tt.contains)
		}
	}
}

func TestDisplayDuration_RunningUsesElapsed(t *testing.T) {
	startedAt := time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339)
	got := DisplayDuration("running", startedAt, 0)
	if got == "0ms" {
		t.Fatalf("DisplayDuration() = %q, want non-zero elapsed time", got)
	}
}

func TestDisplayDuration_FinishedUsesStoredDuration(t *testing.T) {
	got := DisplayDuration("failed", time.Now().UTC().Format(time.RFC3339), 30_000)
	if got != "30s" {
		t.Fatalf("DisplayDuration() = %q, want 30s", got)
	}
}

func TestReadScriptSnapshot(t *testing.T) {
	runDir := t.TempDir()
	content := "echo hello\n"
	if err := os.WriteFile(filepath.Join(runDir, "script.04.sh"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := ReadScriptSnapshot(runDir)
	if err != nil {
		t.Fatalf("ReadScriptSnapshot() error = %v", err)
	}
	if snapshot.Name != "04.sh" {
		t.Errorf("Name = %q, want 04.sh", snapshot.Name)
	}
	if snapshot.Content != content {
		t.Errorf("Content = %q, want %q", snapshot.Content, content)
	}
	if !strings.HasSuffix(snapshot.Path, filepath.Join(runDir, "script.04.sh")) {
		t.Errorf("Path = %q, want script.04.sh under run dir", snapshot.Path)
	}
}

func TestReadScriptSnapshot_Missing(t *testing.T) {
	_, err := ReadScriptSnapshot(t.TempDir())
	if err == nil {
		t.Fatal("ReadScriptSnapshot() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "未找到") {
		t.Errorf("error = %q, want contain 未找到", err.Error())
	}
}
