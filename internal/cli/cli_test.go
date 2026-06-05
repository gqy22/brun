package cli

import (
	"bytes"
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
