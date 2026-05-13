package internal

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Run struct {
	ID         string
	Name       string
	Project    string
	CWD        string
	Command    string
	Status     string
	ExitCode   int
	StartedAt  string
	EndedAt    string
	DurationMs int64
	RunDir     string
	Hostname   string
	Username   string
	GitRepo    string
	GitBranch  string
	GitCommit  string
	GitDirty   bool
}

type Artifact struct {
	ID            int64
	RunID         string
	Kind          string
	Status        string
	Path          string
	AbsPath       string
	StoredPath    string
	Size          int64
	SHA256        string
	Mtime         string
	CaptureMethod string
}

func NewStore(path string) (*Store, error) {
	dir := dirOf(path)
	os.MkdirAll(dir, 0755)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS runs (
			id TEXT PRIMARY KEY,
			name TEXT, project TEXT, cwd TEXT NOT NULL,
			command TEXT NOT NULL, status TEXT NOT NULL,
			exit_code INTEGER, started_at TEXT NOT NULL,
			ended_at TEXT, duration_ms INTEGER,
			run_dir TEXT NOT NULL, hostname TEXT,
			username TEXT, git_repo TEXT, git_branch TEXT,
			git_commit TEXT, git_dirty INTEGER DEFAULT 0,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_runs_started_at ON runs(started_at);`,
		`CREATE INDEX IF NOT EXISTS idx_runs_project ON runs(project);`,
		`CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL, kind TEXT NOT NULL,
			status TEXT, path TEXT NOT NULL,
			abs_path TEXT, stored_path TEXT,
			size_bytes INTEGER, sha256 TEXT,
			mtime TEXT, capture_method TEXT,
			created_at TEXT NOT NULL,
			FOREIGN KEY(run_id) REFERENCES runs(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_run_id ON artifacts(run_id);`,
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL, tag TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(run_id) REFERENCES runs(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tags_run_id ON tags(run_id);`,
		`CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL, note TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(run_id) REFERENCES runs(id)
		);`,
	}
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) CreateRun(r *Run) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO runs (id,name,project,cwd,command,status,exit_code,started_at,ended_at,duration_ms,run_dir,hostname,username,git_repo,git_branch,git_commit,git_dirty,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Name, r.Project, r.CWD, r.Command, r.Status, r.ExitCode,
		r.StartedAt, r.EndedAt, r.DurationMs,
		r.RunDir, r.Hostname, r.Username, r.GitRepo, r.GitBranch, r.GitCommit, b2i(r.GitDirty),
		now, now,
	)
	return err
}

func (s *Store) GetRun(id string) (*Run, error) {
	r := &Run{}
	err := s.db.QueryRow(
		`SELECT id,name,project,cwd,command,status,exit_code,started_at,ended_at,duration_ms,run_dir,hostname,username,git_repo,git_branch,git_commit,git_dirty FROM runs WHERE id=?`, id,
	).Scan(&r.ID, &r.Name, &r.Project, &r.CWD, &r.Command, &r.Status, &r.ExitCode,
		&r.StartedAt, &r.EndedAt, &r.DurationMs, &r.RunDir, &r.Hostname, &r.Username,
		&r.GitRepo, &r.GitBranch, &r.GitCommit, &r.GitDirty)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run %q not found", id)
	}
	return r, err
}

func (s *Store) UpdateRunStatus(id, status string, exitCode int, endedAt string, durationMs int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE runs SET status=?, exit_code=?, ended_at=?, duration_ms=?, updated_at=? WHERE id=?`,
		status, exitCode, endedAt, durationMs, now, id,
	)
	return err
}

func (s *Store) ListRuns(limit int, project, status, tag string) ([]*Run, error) {
	q := `SELECT id,name,project,cwd,command,status,exit_code,started_at,ended_at,duration_ms,run_dir,hostname,username,git_repo,git_branch,git_commit,git_dirty FROM runs WHERE 1=1`
	args := []any{}

	if project != "" {
		q += " AND project=?"
		args = append(args, project)
	}
	if status != "" {
		q += " AND status=?"
		args = append(args, status)
	}
	if tag != "" {
		q += ` AND id IN (SELECT run_id FROM tags WHERE tag=?)`
		args = append(args, tag)
	}
	q += " ORDER BY started_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		r := &Run{}
		err := rows.Scan(&r.ID, &r.Name, &r.Project, &r.CWD, &r.Command, &r.Status, &r.ExitCode,
			&r.StartedAt, &r.EndedAt, &r.DurationMs, &r.RunDir, &r.Hostname, &r.Username,
			&r.GitRepo, &r.GitBranch, &r.GitCommit, &r.GitDirty)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

func (s *Store) GetLatestRun() (*Run, error) {
	rows, err := s.ListRuns(1, "", "", "")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no runs found")
	}
	return rows[0], nil
}

func (s *Store) AddTag(runID, tag string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO tags (run_id,tag,created_at) VALUES(?,?,?)`, runID, tag, now)
	return err
}

func (s *Store) GetTags(runID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT tag FROM tags WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func (s *Store) AddNote(runID, note string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	s.db.Exec(`DELETE FROM notes WHERE run_id=?`, runID)
	_, err := s.db.Exec(`INSERT INTO notes (run_id,note,created_at) VALUES(?,?,?)`, runID, note, now)
	return err
}

func (s *Store) GetNote(runID string) (string, error) {
	var note string
	err := s.db.QueryRow(`SELECT note FROM notes WHERE run_id=? ORDER BY created_at DESC LIMIT 1`, runID).Scan(&note)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return note, err
}

func (s *Store) CreateArtifact(a *Artifact) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`INSERT INTO artifacts (run_id,kind,status,path,abs_path,stored_path,size_bytes,sha256,mtime,capture_method,created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		a.RunID, a.Kind, a.Status, a.Path, a.AbsPath, a.StoredPath, a.Size, a.SHA256, a.Mtime, a.CaptureMethod, now,
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	a.ID = id
	return nil
}

func (s *Store) GetArtifacts(runID string) ([]*Artifact, error) {
	rows, err := s.db.Query(
		`SELECT id,run_id,kind,status,path,abs_path,stored_path,size_bytes,sha256,mtime,capture_method FROM artifacts WHERE run_id=? ORDER BY id`, runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var arts []*Artifact
	for rows.Next() {
		a := &Artifact{}
		if err := rows.Scan(&a.ID, &a.RunID, &a.Kind, &a.Status, &a.Path, &a.AbsPath, &a.StoredPath, &a.Size, &a.SHA256, &a.Mtime, &a.CaptureMethod); err != nil {
			return nil, err
		}
		arts = append(arts, a)
	}
	return arts, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
