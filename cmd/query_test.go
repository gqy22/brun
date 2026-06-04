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
		{ID: "r1", Name: "test-1", Project: "p1", Status: "success", Duration: "1m23s", Command: "echo hi"},
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
}

func TestFormatRunList_Empty(t *testing.T) {
	output := FormatRunList(nil)
	if output == "" {
		t.Error("empty list should still return header or message")
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

func TestResolveRunID_Latest(t *testing.T) {
	// "latest" 应该被特殊处理
	id, isLatest := ResolveRunID("latest")
	if !isLatest {
		t.Error("latest should be detected as latest flag")
	}
	if id != "" {
		t.Errorf("latest id should be empty, got %q", id)
	}
}

func TestResolveRunID_Specific(t *testing.T) {
	id, isLatest := ResolveRunID("20260513-153012-a8f3c2")
	if isLatest {
		t.Error("specific ID should not be latest")
	}
	if id != "20260513-153012-a8f3c2" {
		t.Errorf("id = %q, want 20260513-153012-a8f3c2", id)
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
