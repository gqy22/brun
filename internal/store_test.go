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
		ID:                "20260513-153012-a8f3c2",
		Name:              "test-run",
		Project:           "test-project",
		CWD:               "/home/user/project",
		Command:           "python script.py",
		Status:            "running",
		StartedAt:         time.Now().UTC().Format(time.RFC3339),
		RunDir:            "/tmp/runs/2026/05/13/20260513-153012-a8f3c2",
		Hostname:          "devbox",
		HostnameStatus:    "ok",
		Username:          "user",
		UsernameStatus:    "ok",
		CondaStatus:       "ok",
		CondaEnv:          "rnaseq",
		CondaPrefix:       "/opt/conda/envs/rnaseq",
		PythonVersion:     "Python 3.11.8",
		ResourceSupported: true,
		ResourceStatus:    "ok",
		PeakRSSKB:         1024,
		CPUTimeMs:         25,
		CWDSource:         "explicit",
		ProjectSource:     "config",
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
	if got.CondaStatus != "ok" || got.CondaEnv != "rnaseq" || got.PythonVersion != "Python 3.11.8" {
		t.Errorf("conda = %+v, want ok/rnaseq/Python 3.11.8", got)
	}
	if !got.ResourceSupported || got.ResourceStatus != "ok" || got.PeakRSSKB != 1024 || got.CPUTimeMs != 25 {
		t.Errorf("resources = %+v, want supported ok 1024/25", got)
	}
	if got.Hostname != "devbox" || got.HostnameStatus != "ok" {
		t.Errorf("hostname = %q/%q, want devbox/ok", got.Hostname, got.HostnameStatus)
	}
	if got.Username != "user" || got.UsernameStatus != "ok" {
		t.Errorf("username = %q/%q, want user/ok", got.Username, got.UsernameStatus)
	}
}

func TestRun_DisplayStatus(t *testing.T) {
	cases := []struct {
		name         string
		status       string
		warningCount int
		want         string
	}{
		{"success no warning", "success", 0, "success"},
		{"success with warning", "success", 2, "success_with_warnings"},
		{"failed no warning", "failed", 0, "failed"},
		{"failed with warning", "failed", 1, "failed_with_warnings"},
		{"cancelled no warning", "cancelled", 0, "cancelled"},
		{"cancelled with warning", "cancelled", 3, "cancelled_with_warnings"},
		{"timed out", "timed_out", 0, "timed_out"},
		{"timed out with warning", "timed_out", 1, "timed_out_with_warnings"},
		{"running", "running", 0, "running"},
		{"running with warning", "running", 1, "running"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Run{Status: tc.status, DiagWarningCount: tc.warningCount}
			if got := r.DisplayStatus(); got != tc.want {
				t.Errorf("DisplayStatus() = %q, want %q", got, tc.want)
			}
		})
	}
	if got := (*Run)(nil).DisplayStatus(); got != "" {
		t.Errorf("nil Run DisplayStatus() = %q, want \"\"", got)
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

	runs, err := ro.ListRuns(10, "", "", "", "", "", "", false, "", "")
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

func TestStore_IncrementRunDiagnostic(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	run := &Run{ID: "diag-inc", Status: "running", CWD: "/t", Command: "c", StartedAt: ts(), RunDir: "/t"}
	if err := s.CreateRun(run); err != nil {
		t.Fatal(err)
	}

	if err := s.IncrementRunDiagnostic("diag-inc", "warning", "metadata_write_failed", "2026-06-11T00:00:00Z"); err != nil {
		t.Fatalf("IncrementRunDiagnostic() error = %v", err)
	}
	if err := s.IncrementRunDiagnostic("diag-inc", "warning", "second", "2026-06-11T00:00:01Z"); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementRunDiagnostic("diag-inc", "error", "boom", "2026-06-11T00:00:02Z"); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementRunDiagnostic("diag-inc", "info", "note", "2026-06-11T00:00:03Z"); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetRun("diag-inc")
	if err != nil {
		t.Fatal(err)
	}
	if got.DiagInfoCount != 1 || got.DiagWarningCount != 2 || got.DiagErrorCount != 1 {
		t.Errorf("counts = info=%d warn=%d err=%d, want 1/2/1", got.DiagInfoCount, got.DiagWarningCount, got.DiagErrorCount)
	}
	// last_code/last_at 应跟随最后一次写入
	if got.DiagLastCode != "note" || got.DiagLastAt != "2026-06-11T00:00:03Z" {
		t.Errorf("last = %+v, want note/2026-06-11T00:00:03Z", got)
	}
	// 关键：计数 > 0 时 display_status 切到 success_with_warnings
	if got.DisplayStatus() != "" && got.Status == "success" && got.DiagWarningCount > 0 {
		// 当前 status=running，所以 display 仍为 running；这里只是确保函数能跑
	}
	// 显式改成 success+warning 验证 display_status 切档
	if err := s.UpdateRunStatus("diag-inc", "success", 0, ts(), 100); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetRun("diag-inc")
	if got.DisplayStatus() != "success_with_warnings" {
		t.Errorf("DisplayStatus = %q, want success_with_warnings", got.DisplayStatus())
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

	runs, err := s.ListRuns(10, "", "", "", "", "", "", false, "", "")
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

	runs, _ := s.ListRuns(10, "alpha", "", "", "", "", "", false, "", "")
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

	runs, _ := s.ListRuns(10, "", "failed", "", "", "", "", false, "", "")
	if len(runs) != 1 || runs[0].ID != "f1" {
		t.Errorf("filter by status failed")
	}
}

func TestStore_ListRuns_FilterByHostAndUser(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	now := ts()
	s.CreateRun(&Run{ID: "devbox-alice", Hostname: "devbox", Username: "alice", Command: "c", Status: "success", StartedAt: now, RunDir: "/t", CWD: "/t"})
	s.CreateRun(&Run{ID: "devbox-bob", Hostname: "devbox", Username: "bob", Command: "c", Status: "success", StartedAt: now, RunDir: "/t", CWD: "/t"})
	s.CreateRun(&Run{ID: "build-alice", Hostname: "build", Username: "alice", Command: "c", Status: "success", StartedAt: now, RunDir: "/t", CWD: "/t"})

	// 仅 host
	byHost, _ := s.ListRuns(10, "", "", "", "", "", "", false, "devbox", "")
	if len(byHost) != 2 {
		t.Errorf("by host: got %d runs, want 2", len(byHost))
	}

	// 仅 user
	byUser, _ := s.ListRuns(10, "", "", "", "", "", "", false, "", "alice")
	if len(byUser) != 2 {
		t.Errorf("by user: got %d runs, want 2", len(byUser))
	}

	// host + user
	both, _ := s.ListRuns(10, "", "", "", "", "", "", false, "devbox", "alice")
	if len(both) != 1 || both[0].ID != "devbox-alice" {
		t.Errorf("by host+user: got %+v, want devbox-alice", both)
	}

	// 组合其它过滤（host + status）
	withStatus, _ := s.ListRuns(10, "", "success", "", "", "", "", false, "build", "alice")
	if len(withStatus) != 1 || withStatus[0].ID != "build-alice" {
		t.Errorf("by host+user+status: got %+v, want build-alice", withStatus)
	}
}

func TestStore_ListRuns_FilterByWithWarnings(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	now := ts()
	s.CreateRun(&Run{ID: "clean", Status: "success", StartedAt: now, RunDir: "/t", CWD: "/t"})
	s.CreateRun(&Run{ID: "warned", Status: "success", StartedAt: now, RunDir: "/t", CWD: "/t"})
	if err := s.UpdateRunDiagnostics("warned", DiagnosticSummary{WarningCount: 1, LastCode: "x", LastAt: now}); err != nil {
		t.Fatal(err)
	}

	// withWarnings=true 命中所有 diag_warning_count>0
	all, err := s.ListRuns(10, "", "", "", "", "", "", true, "", "")
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(all) != 1 || all[0].ID != "warned" {
		t.Fatalf("withWarnings filter failed, got %+v", all)
	}
	if all[0].DisplayStatus() != "success_with_warnings" {
		t.Errorf("DisplayStatus = %q, want success_with_warnings", all[0].DisplayStatus())
	}

	// withWarnings=true 与 status 过滤组合
	byStatus, err := s.ListRuns(10, "", "success", "", "", "", "", true, "", "")
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(byStatus) != 1 || byStatus[0].ID != "warned" {
		t.Fatalf("withWarnings+status filter failed, got %+v", byStatus)
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

func TestStore_MigrateBackfillsEnvStatus(t *testing.T) {
	dir := fastTempDir(t)
	dbPath := filepath.Join(dir, "test.db")

	// 1) 准备一个 v5 状态的老库：schema 升级到当前版本（v7），并模拟两个老 run。
	//    r-ok 已经采集到 hostname/username；r-empty 没有值。
	//    然后把 user_version 拨回 5，模拟"v5 老库"等待升级。
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRun(&Run{ID: "r-ok", Status: "success", RunDir: dir, CWD: dir, StartedAt: ts()}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRun(&Run{ID: "r-empty", Status: "failed", RunDir: dir, CWD: dir, StartedAt: ts()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE runs SET hostname='legacy-host', username='legacy-user', hostname_status=NULL, username_status=NULL WHERE id='r-ok'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE runs SET hostname='', username='', hostname_status=NULL, username_status=NULL WHERE id='r-empty'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version=5`); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}

	// 2) 重新打开：触发 v5→v7 迁移，UPDATE 回填应该把状态从 NULL 写成 ok / unavailable
	s, err = NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// r-ok 已有 hostname/username → *status=ok
	rOK, err := s.GetRun("r-ok")
	if err != nil {
		t.Fatal(err)
	}
	if rOK.HostnameStatus != "ok" {
		t.Errorf("r-ok hostname_status = %q, want ok", rOK.HostnameStatus)
	}
	if rOK.UsernameStatus != "ok" {
		t.Errorf("r-ok username_status = %q, want ok", rOK.UsernameStatus)
	}

	// r-empty 采集失败 → *status=unavailable
	rEmpty, err := s.GetRun("r-empty")
	if err != nil {
		t.Fatal(err)
	}
	if rEmpty.HostnameStatus != "unavailable" {
		t.Errorf("r-empty hostname_status = %q, want unavailable", rEmpty.HostnameStatus)
	}
	if rEmpty.UsernameStatus != "unavailable" {
		t.Errorf("r-empty username_status = %q, want unavailable", rEmpty.UsernameStatus)
	}

	// 3) 幂等：再触发一次迁移，状态应该保持不变
	if err := s.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	rOK2, _ := s.GetRun("r-ok")
	if rOK2.HostnameStatus != "ok" || rOK2.UsernameStatus != "ok" {
		t.Errorf("after re-migrate: r-ok status drifted to %q / %q", rOK2.HostnameStatus, rOK2.UsernameStatus)
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
