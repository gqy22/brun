package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_InitDB(t *testing.T) {
	tmp := t.TempDir()
	dbPath := tmp + "/test.db"

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file not created")
	}
}

func TestStore_CreateRun(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	run := &Run{
		ID:            "20260513-153012-a8f3c2",
		Name:          "test-run",
		Project:       "test-project",
		CWD:           "/home/user/project",
		Command:       "python script.py",
		Status:        "running",
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
		RunDir:        "/tmp/runs/2026/05/13/20260513-153012-a8f3c2",
		Hostname:      "devbox",
		Username:      "user",
		CWDSource:     "explicit",
		ProjectSource: "config",
	}

	err := s.CreateRun(run)
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	got, err := s.GetRun(run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.Project != run.Project {
		t.Errorf("Project = %q, want %q", got.Project, run.Project)
	}
	if got.Command != run.Command {
		t.Errorf("Command = %q, want %q", got.Command, run.Command)
	}
	if got.CWDSource != "explicit" || got.ProjectSource != "config" {
		t.Errorf("sources = %q/%q, want explicit/config", got.CWDSource, got.ProjectSource)
	}
}

func TestOpenStoreReadOnlyAllowsQueriesOnly(t *testing.T) {
	dbPath := filepath.Join(fastTempDir(t), "path with spaces", "test #1.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if err := s.CreateRun(&Run{ID: "ro-1", CWD: "/t", Command: "echo hi", Status: "success", StartedAt: ts(), RunDir: "/t"}); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	s.Close()

	ro, err := OpenStoreReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly() error = %v", err)
	}
	defer ro.Close()

	runs, err := ro.ListRuns(10, "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "ro-1" {
		t.Fatalf("unexpected readonly query result: %+v", runs)
	}

	err = ro.CreateRun(&Run{ID: "ro-2", CWD: "/t", Command: "echo no", Status: "running", StartedAt: ts(), RunDir: "/t"})
	if err == nil {
		t.Fatal("CreateRun() on readonly store succeeded, want error")
	}
}

func TestSQLiteSyncModeUsesEnvironment(t *testing.T) {
	t.Setenv("BRUN_SQLITE_SYNC", "")
	if got := sqliteSyncMode(); got != "OFF" {
		t.Fatalf("default sqliteSyncMode() = %q, want OFF", got)
	}
	t.Setenv("BRUN_SQLITE_SYNC", "normal")
	if got := sqliteSyncMode(); got != "NORMAL" {
		t.Fatalf("sqliteSyncMode() = %q, want NORMAL", got)
	}
	t.Setenv("BRUN_SQLITE_SYNC", "full")
	if got := sqliteSyncMode(); got != "FULL" {
		t.Fatalf("sqliteSyncMode() = %q, want FULL", got)
	}
	t.Setenv("BRUN_SQLITE_SYNC", "bad")
	if got := sqliteSyncMode(); got != "OFF" {
		t.Fatalf("invalid sqliteSyncMode() = %q, want OFF", got)
	}
}

func TestStore_UpdateRunStatus(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	run := &Run{ID: "test-001", Status: "running", CWD: "/tmp", Command: "echo hi", StartedAt: time.Now().UTC().Format(time.RFC3339), RunDir: "/tmp/r"}
	s.CreateRun(run)

	endedAt := time.Now().UTC().Format(time.RFC3339)
	err := s.UpdateRunStatus("test-001", "success", 0, endedAt, 30_000)
	if err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}

	got, _ := s.GetRun("test-001")
	if got.Status != "success" {
		t.Errorf("Status = %q, want %q", got.Status, "success")
	}
	if got.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", got.ExitCode)
	}
	if got.DurationMs != 30_000 {
		t.Errorf("DurationMs = %d, want 30000", got.DurationMs)
	}
}

func TestStore_UpdateRunDiagnostics(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	run := &Run{ID: "diag-001", Status: "running", CWD: "/tmp", Command: "echo hi", StartedAt: ts(), RunDir: "/tmp/r"}
	if err := s.CreateRun(run); err != nil {
		t.Fatal(err)
	}

	err := s.UpdateRunDiagnostics("diag-001", DiagnosticSummary{
		InfoCount:    3,
		WarningCount: 2,
		ErrorCount:   1,
		LastCode:     "metadata_write_failed",
		LastAt:       "2026-06-05T01:02:03Z",
	})
	if err != nil {
		t.Fatalf("UpdateRunDiagnostics() error = %v", err)
	}

	got, err := s.GetRun("diag-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.DiagInfoCount != 3 || got.DiagWarningCount != 2 || got.DiagErrorCount != 1 || got.DiagLastCode != "metadata_write_failed" {
		t.Fatalf("diagnostics = %+v", got)
	}
}

func TestStore_ListRuns(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	base := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 3; i++ {
		s.CreateRun(&Run{
			ID:        runID(i),
			Project:   "proj-a",
			Command:   "cmd",
			Status:    "success",
			StartedAt: base,
			RunDir:    "/tmp/r",
			CWD:       "/tmp",
		})
	}

	runs, err := s.ListRuns(10, "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("ListRuns() returned %d runs, want 3", len(runs))
	}
}

func TestStore_ListRuns_FilterByProject(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	now := ts()
	s.CreateRun(&Run{ID: "p1", Project: "alpha", Command: "c", Status: "success", StartedAt: now, RunDir: "/t", CWD: "/t"})
	s.CreateRun(&Run{ID: "p2", Project: "beta", Command: "c", Status: "success", StartedAt: now, RunDir: "/t", CWD: "/t"})

	runs, _ := s.ListRuns(10, "alpha", "", "", "", "", "")
	if len(runs) != 1 || runs[0].ID != "p1" {
		t.Errorf("filter by project failed, got %d runs", len(runs))
	}
}

func TestStore_ListRuns_FilterByStatus(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	now := ts()
	s.CreateRun(&Run{ID: "s1", Project: "p", Command: "c", Status: "success", StartedAt: now, RunDir: "/t", CWD: "/t"})
	s.CreateRun(&Run{ID: "f1", Project: "p", Command: "c", Status: "failed", StartedAt: now, RunDir: "/t", CWD: "/t"})

	runs, _ := s.ListRuns(10, "", "failed", "", "", "", "")
	if len(runs) != 1 || runs[0].ID != "f1" {
		t.Errorf("filter by status failed")
	}
}

func TestStore_AddTag(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	s.CreateRun(&Run{ID: "t1", CWD: "/t", Command: "c", StartedAt: time.Now().UTC().Format(time.RFC3339), RunDir: "/t"})

	err := s.AddTag("t1", "important")
	if err != nil {
		t.Fatalf("AddTag() error = %v", err)
	}

	tags, err := s.GetTags("t1")
	if err != nil {
		t.Fatalf("GetTags() error = %v", err)
	}
	if len(tags) != 1 || tags[0] != "important" {
		t.Errorf("tags = %v, want [important]", tags)
	}
}

func TestStore_AddNote(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	s.CreateRun(&Run{ID: "n1", CWD: "/t", Command: "c", StartedAt: ts(), RunDir: "/t"})

	err := s.AddNote("n1", "this is a note")
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}

	note, err := s.GetNote("n1")
	if err != nil {
		t.Fatalf("GetNote() error = %v", err)
	}
	if note != "this is a note" {
		t.Errorf("note = %q, want %q", note, "this is a note")
	}
}

func TestStore_CreateArtifact(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	s.CreateRun(&Run{ID: "a1", CWD: "/t", Command: "c", StartedAt: ts(), RunDir: "/t"})

	a := &Artifact{
		RunID:   "a1",
		Kind:    "output",
		Status:  "created",
		Path:    "results/out.bam",
		AbsPath: "/t/results/out.bam",
		Size:    8_400_000_000,
	}

	err := s.CreateArtifact(a)
	if err != nil {
		t.Fatalf("CreateArtifact() error = %v", err)
	}

	arts, err := s.GetArtifacts("a1")
	if err != nil {
		t.Fatalf("GetArtifacts() error = %v", err)
	}
	if len(arts) != 1 || arts[0].Path != "results/out.bam" {
		t.Errorf("artifact mismatch: %+v", arts)
	}
}

func TestStore_GetLatestRun(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	now := ts()
	s.CreateRun(&Run{ID: "old", CWD: "/t", Command: "old cmd", Status: "success", StartedAt: now, RunDir: "/t"})
	s.CreateRun(&Run{ID: "new", CWD: "/t", Command: "new cmd", Status: "running", StartedAt: ts(), RunDir: "/t"})

	latest, err := s.GetLatestRun()
	if err != nil {
		t.Fatalf("GetLatestRun() error = %v", err)
	}
	if latest.ID != "new" {
		t.Errorf("latest ID = %q, want %q", latest.ID, "new")
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(fastTempDir(t), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	return s
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

func runID(i int) string {
	return fmt.Sprintf("run-%03d", i)
}

func ts() string {
	return time.Now().UTC().Format(time.RFC3339)
}
