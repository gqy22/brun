package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biotools/brun/internal"
)

func TestScriptCmdPrintsSnapshot(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)

	runID := "20260522-153012-a8f3c2"
	runDir := filepath.Join(home, "runs", "2026", "05", "22", runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "echo hello\n"
	if err := os.WriteFile(filepath.Join(runDir, "script.04.sh"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CreateRun(&internal.Run{
		ID:        runID,
		Project:   "proj",
		CWD:       "/tmp",
		Command:   "bash 04.sh",
		Status:    "success",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		RunDir:    runDir,
	}); err != nil {
		t.Fatal(err)
	}

	c := scriptCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{runID})

	if err := c.Execute(); err != nil {
		t.Fatalf("scriptCmd() error = %v", err)
	}
	if out.String() != content {
		t.Errorf("output = %q, want %q", out.String(), content)
	}
}

func TestScriptCmdPathFlag(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)

	runID := "20260522-153012-a8f3c2"
	runDir := filepath.Join(home, "runs", "2026", "05", "22", runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(runDir, "script.04.sh")
	if err := os.WriteFile(scriptPath, []byte("echo hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CreateRun(&internal.Run{
		ID:        runID,
		CWD:       "/tmp",
		Command:   "bash 04.sh",
		Status:    "success",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		RunDir:    runDir,
	}); err != nil {
		t.Fatal(err)
	}

	c := scriptCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{runID, "--path"})

	if err := c.Execute(); err != nil {
		t.Fatalf("scriptCmd() error = %v", err)
	}
	if strings.TrimSpace(out.String()) != scriptPath {
		t.Errorf("output = %q, want %q", strings.TrimSpace(out.String()), scriptPath)
	}
}

func TestScriptCmdRequiresRunSelector(t *testing.T) {
	c := scriptCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(nil)

	if err := c.Execute(); err == nil {
		t.Fatal("scriptCmd() expected selector error")
	}
}

func TestDiagCmdShowsWarningsByDefault(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)

	runID := "20260605-153012-diag01"
	runDir := filepath.Join(home, "runs", "2026", "06", "05", runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	writer := internal.NewDiagnosticWriter(runDir)
	writer.Info("project_inferred", "已推断项目名", "proj")
	writer.Warning("metadata_write_failed", "metadata.yaml 写入失败", "disk full")

	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CreateRun(&internal.Run{
		ID:        runID,
		CWD:       "/tmp",
		Command:   "echo hi",
		Status:    "success",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		RunDir:    runDir,
	}); err != nil {
		t.Fatal(err)
	}

	c := diagCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{runID})

	if err := c.Execute(); err != nil {
		t.Fatalf("diagCmd() error = %v", err)
	}
	if strings.Contains(out.String(), "project_inferred") {
		t.Fatalf("diag output should hide info by default: %s", out.String())
	}
	if !strings.Contains(out.String(), "metadata_write_failed") {
		t.Fatalf("diag output missing warning: %s", out.String())
	}
}

func TestShowCmdJSONOutput(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)

	runID := "20260605-153012-json01"
	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.CreateRun(&internal.Run{
		ID:            runID,
		Project:       "proj",
		ProjectSource: "explicit",
		CWD:           "/tmp",
		CWDSource:     "explicit",
		Command:       "echo hi",
		Status:        "success",
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
		RunDir:        filepath.Join(home, "runs", "x"),
	}); err != nil {
		t.Fatal(err)
	}

	c := showCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{runID, "--json"})

	if err := c.Execute(); err != nil {
		t.Fatalf("showCmd() error = %v", err)
	}
	var resp struct {
		ID            string `json:"id"`
		ProjectSource string `json:"project_source"`
		CWDSource     string `json:"cwd_source"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v\n%s", err, out.String())
	}
	if resp.ID != runID || resp.ProjectSource != "explicit" || resp.CWDSource != "explicit" {
		t.Fatalf("unexpected show json: %+v", resp)
	}
}

func TestFormatCLIErrorIncludesCodeAndHint(t *testing.T) {
	err := cliError("invalid_time_filter", "无效时间", "使用 today", nil)
	out := formatCLIError(err)
	if !strings.Contains(out, "Code: invalid_time_filter") || !strings.Contains(out, "Hint: 使用 today") {
		t.Fatalf("formatted error missing code/hint: %s", out)
	}
}

func TestExecuteRunWritesDiagnostics(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	cwd := fastTempDir(t)

	if err := executeRun([]string{"sh", "-c", "true"}, "", "", "", nil, true, "", 0, cwd, ""); err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run, err := store.GetLatestRun()
	if err != nil {
		t.Fatalf("GetLatestRun() error = %v", err)
	}
	events, err := internal.ReadDiagnostics(run.RunDir)
	if err != nil {
		t.Fatalf("ReadDiagnostics() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected diagnostics events")
	}
	var sawProjectInferred, sawScriptMissing bool
	for _, event := range events {
		if event.Code == "project_inferred" {
			sawProjectInferred = true
		}
		if event.Code == "script_snapshot_missing" {
			sawScriptMissing = true
		}
	}
	if !sawProjectInferred {
		t.Fatalf("missing project_inferred event: %+v", events)
	}
	if !sawScriptMissing {
		t.Fatalf("missing script_snapshot_missing event: %+v", events)
	}
}

func TestExecuteRunFailsOnInvalidConfig(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	cwd := fastTempDir(t)
	if err := os.WriteFile(filepath.Join(cwd, "brun.yaml"), []byte("project: [bad\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := executeRun([]string{"sh", "-c", "true"}, "", "", "", nil, true, "", 0, cwd, "")
	if err == nil || !strings.Contains(err.Error(), "项目配置错误") {
		t.Fatalf("executeRun() error = %v, want config error", err)
	}
}

func TestParseTimeFilterRejectsInvalidInput(t *testing.T) {
	if _, err := parseTimeFilter("yesterday-ish"); err == nil {
		t.Fatal("parseTimeFilter() expected error")
	}
	if _, err := parseTimeFilter("0d"); err == nil {
		t.Fatal("parseTimeFilter() expected error for 0d")
	}
}

func TestParseTimeFilterAcceptsSupportedInputs(t *testing.T) {
	inputs := []string{"2026-06-05", "2026-06-05T01:02:03Z", "today", "1h", "2d", "3w"}
	for _, input := range inputs {
		if got, err := parseTimeFilter(input); err != nil || got == "" {
			t.Fatalf("parseTimeFilter(%q) = %q, %v", input, got, err)
		}
	}
}

func TestExecuteRunUsesProvidedRunID(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	cwd := fastTempDir(t)
	runID := "20260605-091500-fixed1"

	if err := executeRun([]string{"sh", "-c", "true"}, "", "", "", nil, true, "", 0, cwd, runID); err != nil {
		t.Fatalf("executeRun() error = %v", err)
	}

	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run, err := store.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun(%q) error = %v", runID, err)
	}
	if run.RunDir != internal.RunDir(runID) {
		t.Fatalf("RunDir = %q, want %q", run.RunDir, internal.RunDir(runID))
	}
}

func TestReadRunMetadata(t *testing.T) {
	runDir := fastTempDir(t)
	path := filepath.Join(runDir, "metadata.yaml")
	data := []byte("id: r1\nproject: p\ncommand: echo hi\nstatus: success\nexit_code: 0\ncwd: /tmp\nstarted_at: 2026-06-05T01:00:00Z\nended_at: 2026-06-05T01:00:01Z\nduration_ms: 1000\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	run, err := readRunMetadata(path)
	if err != nil {
		t.Fatalf("readRunMetadata() error = %v", err)
	}
	if run.ID != "r1" || run.Project != "p" || run.Command != "echo hi" || run.DurationMs != 1000 {
		t.Fatalf("unexpected run metadata: %+v", run)
	}
}

func TestLoadRunsFromMetadata(t *testing.T) {
	root := fastTempDir(t)
	runDir := filepath.Join(root, "2026", "06", "05", "r1")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "metadata.yaml"), []byte("id: r1\ncommand: echo hi\nstatus: success\ncwd: /tmp\nstarted_at: 2026-06-05T01:00:00Z\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runs, err := loadRunsFromMetadata(root)
	if err != nil {
		t.Fatalf("loadRunsFromMetadata() error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "r1" || runs[0].RunDir != runDir {
		t.Fatalf("runs = %+v, want r1 with runDir", runs)
	}
}

func fastTempDir(t *testing.T) string {
	t.Helper()
	base := "/dev/shm"
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		base = ""
	}
	dir, err := os.MkdirTemp(base, "brun-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
