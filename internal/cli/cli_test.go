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
