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
	"github.com/spf13/cobra"
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
	output := out.String()
	// Should contain the metadata header followed by script content
	if !strings.Contains(output, "04.sh") || !strings.Contains(output, "echo hello") {
		t.Errorf("output = %q, want header with 04.sh and content echo hello", output)
	}
	// Header line starts with # ──
	if !strings.Contains(output, "# ──") {
		t.Errorf("output missing metadata header, got: %q", output)
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
		ID:                runID,
		Project:           "proj",
		ProjectSource:     "explicit",
		CWD:               "/tmp",
		CWDSource:         "explicit",
		Command:           "echo hi",
		Status:            "success",
		StartedAt:         time.Now().UTC().Format(time.RFC3339),
		RunDir:            filepath.Join(home, "runs", "x"),
		CondaStatus:       "ok",
		CondaEnv:          "rnaseq",
		CondaPrefix:       "/opt/conda/envs/rnaseq",
		PythonVersion:     "Python 3.11.8",
		ResourceSupported: true,
		ResourceStatus:    "ok",
		PeakRSSKB:         1024,
		CPUTimeMs:         25,
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
		ID                string `json:"id"`
		ProjectSource     string `json:"project_source"`
		CWDSource         string `json:"cwd_source"`
		CondaStatus       string `json:"conda_status"`
		CondaEnv          string `json:"conda_env"`
		PythonVersion     string `json:"python_version"`
		ResourceSupported bool   `json:"resource_supported"`
		ResourceStatus    string `json:"resource_status"`
		PeakRSSKB         int64  `json:"peak_rss_kb"`
		CPUTimeMs         int64  `json:"cpu_time_ms"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v\n%s", err, out.String())
	}
	if resp.ID != runID || resp.ProjectSource != "explicit" || resp.CWDSource != "explicit" {
		t.Fatalf("unexpected show json: %+v", resp)
	}
	if resp.CondaStatus != "ok" || resp.CondaEnv != "rnaseq" || resp.PythonVersion != "Python 3.11.8" {
		t.Fatalf("unexpected conda json: %+v", resp)
	}
	if !resp.ResourceSupported || resp.ResourceStatus != "ok" || resp.PeakRSSKB != 1024 || resp.CPUTimeMs != 25 {
		t.Fatalf("unexpected resource json: %+v", resp)
	}
}

func TestFormatCLIErrorIncludesCodeAndHint(t *testing.T) {
	err := cliError("invalid_time_filter", "无效时间", "使用 today", nil)
	out := formatCLIError(err)
	if !strings.Contains(out, "Code: invalid_time_filter") || !strings.Contains(out, "Hint: 使用 today") {
		t.Fatalf("formatted error missing code/hint: %s", out)
	}
}

func TestRootCommandRequiresDashDashForShortcutRun(t *testing.T) {
	err := cliError("missing_command_separator", "未知命令或缺少 -- 分隔符", "运行命令请使用 brun -- <command>", nil)
	out := formatCLIError(err)
	if !strings.Contains(out, "Code: missing_command_separator") {
		t.Fatalf("formatted error missing shortcut code: %s", out)
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

func TestExecuteRunRecordsMissingCommandAsFailed(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	cwd := fastTempDir(t)

	if err := executeRun([]string{"nonexistent_command_abc123"}, "bad-cmd", "error-test", "", nil, true, "", 0, cwd, ""); err != nil {
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
	if run.Status != "failed" || run.ExitCode != 127 {
		t.Fatalf("run status/exit = %q/%d, want failed/127", run.Status, run.ExitCode)
	}
	if !strings.Contains(run.Command, "nonexistent_command_abc123") {
		t.Fatalf("run command = %q, want missing command name", run.Command)
	}
	if _, err := os.Stat(filepath.Join(run.RunDir, "metadata.yaml")); err != nil {
		t.Fatalf("metadata.yaml missing: %v", err)
	}
	stderrData, err := os.ReadFile(filepath.Join(run.RunDir, "stderr.er"))
	if err != nil {
		t.Fatalf("stderr.er missing: %v", err)
	}
	if !strings.Contains(string(stderrData), "nonexistent_command_abc123") {
		t.Fatalf("stderr.er missing command name: %s", string(stderrData))
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
	data := []byte("id: r1\nproject: p\ncommand: echo hi\nstatus: success\nexit_code: 0\ncwd: /tmp\nstarted_at: 2026-06-05T01:00:00Z\nended_at: 2026-06-05T01:00:01Z\nduration_ms: 1000\nconda_status: ok\nconda_env: rnaseq\nconda_prefix: /opt/conda/envs/rnaseq\npython_version: Python 3.11.8\nresource_supported: true\nresource_status: ok\npeak_rss_kb: 1024\ncpu_time_ms: 25\n")
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
	if run.CondaStatus != "ok" || run.CondaEnv != "rnaseq" || run.PythonVersion != "Python 3.11.8" {
		t.Fatalf("unexpected conda metadata: %+v", run)
	}
	if !run.ResourceSupported || run.ResourceStatus != "ok" || run.PeakRSSKB != 1024 || run.CPUTimeMs != 25 {
		t.Fatalf("unexpected resource metadata: %+v", run)
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

func TestCleanCmdDryRunJSONDoesNotDelete(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	runDir := filepath.Join(home, "runs", "old-run")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stdout.o"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	if err := store.CreateRun(&internal.Run{
		ID:        "old-run",
		CWD:       "/tmp",
		Command:   "echo hi",
		Status:    "success",
		StartedAt: startedAt,
		RunDir:    runDir,
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	c := cleanCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{"--older-than", "1d", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatalf("cleanCmd() error = %v", err)
	}
	var resp struct {
		Write bool `json:"write"`
		Count int  `json:"count"`
		Runs  []struct {
			RunID string `json:"run_id"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "\n  \"count\"") {
		t.Fatalf("clean JSON should use the shared indented format: %s", out.String())
	}
	if resp.Write || resp.Count != 1 || len(resp.Runs) != 1 || resp.Runs[0].RunID != "old-run" {
		t.Fatalf("unexpected clean json: %+v", resp)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Fatalf("dry-run removed run dir: %v", err)
	}
}

func TestCleanCmdWriteDeletesMatchedRun(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	runDir := filepath.Join(home, "runs", "old-run")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "stdout.o"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	if err := store.CreateRun(&internal.Run{
		ID:        "old-run",
		CWD:       "/tmp",
		Command:   "echo hi",
		Status:    "success",
		StartedAt: startedAt,
		RunDir:    runDir,
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	c := cleanCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{"--older-than", "1d", "--write"})
	if err := c.Execute(); err != nil {
		t.Fatalf("cleanCmd() error = %v", err)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("run dir still exists after clean --write: %v", err)
	}

	store, err = openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.GetRun("old-run"); err == nil {
		t.Fatal("run still exists after clean --write")
	}
}

func TestListCmdFiltersDisplayStatus(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, run := range []*internal.Run{
		{ID: "plain", CWD: "/tmp", Command: "true", Status: "success", StartedAt: now, RunDir: home},
		{ID: "warned", CWD: "/tmp", Command: "true", Status: "success", StartedAt: now, RunDir: home, DiagWarningCount: 1},
	} {
		if err := store.CreateRun(run); err != nil {
			t.Fatal(err)
		}
	}
	store.Close()

	c := listCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"--status", "success_with_warnings", "--json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("decode list JSON: %v\n%s", err, out.String())
	}
	if len(rows) != 1 || rows[0].ID != "warned" {
		t.Fatalf("rows = %+v, want only warned", rows)
	}
}

func TestParseListStatusFilterAcceptsTimedOut(t *testing.T) {
	base, withWarnings, err := parseListStatusFilter("timed_out_with_warnings")
	if err != nil {
		t.Fatal(err)
	}
	if base != "timed_out" || !withWarnings {
		t.Fatalf("base=%q withWarnings=%t", base, withWarnings)
	}
}

func TestCleanCmdAcceptsAbsoluteDate(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(&internal.Run{
		ID: "old", CWD: "/tmp", Command: "true", Status: "success",
		StartedAt: time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339), RunDir: home,
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	c := cleanCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"--older-than", time.Now().Add(-24 * time.Hour).UTC().Format("2006-01-02"), "--json"})
	if err := c.Execute(); err != nil {
		t.Fatalf("clean absolute date: %v", err)
	}
	var payload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil || payload.Count != 1 {
		t.Fatalf("payload = %+v, err = %v, output = %s", payload, err, out.String())
	}
}

func TestRerunWithSameTagsCopiesTags(t *testing.T) {
	home := fastTempDir(t)
	cwd := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(&internal.Run{
		ID: "source", Project: "proj", CWD: cwd, Command: "true", Status: "success",
		StartedAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), RunDir: home,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTag("source", "sample:S1"); err != nil {
		t.Fatal(err)
	}
	store.Close()

	c := rerunCmd()
	c.SetArgs([]string{"source", "--with-same-tags", "--name", "copied"})
	if err := c.Execute(); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	store, err = openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	latest, err := store.GetLatestRun()
	if err != nil {
		t.Fatal(err)
	}
	tags, err := store.GetTags(latest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Name != "copied" || !containsString(tags, "sample:S1") {
		t.Fatalf("latest = %+v, tags = %v", latest, tags)
	}
}

func TestCommandArgumentContracts(t *testing.T) {
	for name, factory := range map[string]func() *cobra.Command{
		"list": listCmd, "clean": cleanCmd, "repair": repairCmd, "web": webCmd,
	} {
		t.Run(name, func(t *testing.T) {
			c := factory()
			c.SetArgs([]string{"stray"})
			if err := c.Execute(); err == nil {
				t.Fatal("unexpected positional argument was accepted")
			}
		})
	}
}

func TestLogsAndScriptFlagContracts(t *testing.T) {
	logs := logsCmd()
	if flag := logs.Flags().ShorthandLookup("f"); flag == nil || flag.Name != "follow" {
		t.Fatalf("logs -f is not registered as --follow: %+v", flag)
	}
	logs.SetArgs([]string{"id", "--stdout", "--stderr"})
	if err := logs.Execute(); err == nil || !strings.Contains(err.Error(), "不能同时") {
		t.Fatalf("logs accepted mutually exclusive streams: %v", err)
	}

	script := scriptCmd()
	script.SetArgs([]string{"id1", "id2", "--path"})
	if err := script.Execute(); err == nil || !strings.Contains(err.Error(), "--path") {
		t.Fatalf("script accepted --path in diff mode: %v", err)
	}
}

func TestListRejectsInvalidLimitAndStatus(t *testing.T) {
	for name, args := range map[string][]string{
		"zero limit":     {"--limit", "0"},
		"unknown status": {"--status", "done"},
	} {
		t.Run(name, func(t *testing.T) {
			home := fastTempDir(t)
			t.Setenv("BRUN_HOME", home)
			c := listCmd()
			c.SetArgs(args)
			if err := c.Execute(); err == nil {
				t.Fatalf("list accepted invalid args %v", args)
			}
		})
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

func TestHostnameHelperReturnsStatus(t *testing.T) {
	val, status := hostname()
	if val == "" {
		if status != "unavailable" {
			t.Errorf("empty hostname should report unavailable, got %q", status)
		}
	} else {
		if status != "ok" {
			t.Errorf("non-empty hostname should report ok, got %q", status)
		}
	}
}

func TestUsernameHelperReportsUnavailableWhenUSERMissing(t *testing.T) {
	t.Setenv("USER", "")
	val, status := username()
	if val != "" || status != "unavailable" {
		t.Errorf("username = %q/%q, want empty/unavailable", val, status)
	}

	t.Setenv("USER", "alice")
	val, status = username()
	if val != "alice" || status != "ok" {
		t.Errorf("username = %q/%q, want alice/ok", val, status)
	}
}
