package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfig_Full(t *testing.T) {
	yaml := `
project: rnaseq-dev
capture:
  scripts:
    - "scripts/**/*.py"
  outputs:
    - "results/**/*"
ignore:
  - ".git/**"
  - "tmp/**"
artifacts:
  copy:
    - "reports/**/*.html"
  metadata_only:
    - "data/**/*.fastq.gz"
`
	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.Project != "rnaseq-dev" {
		t.Errorf("Project = %q, want rnaseq-dev", cfg.Project)
	}
	if len(cfg.Capture.Scripts) != 1 || cfg.Capture.Scripts[0] != "scripts/**/*.py" {
		t.Errorf("Scripts = %v", cfg.Capture.Scripts)
	}
	if len(cfg.Ignore) != 2 {
		t.Errorf("Ignore count = %d, want 2", len(cfg.Ignore))
	}
}

func TestParseConfig_Empty(t *testing.T) {
	cfg, err := ParseConfig([]byte{})
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.Project != "" {
		t.Errorf("empty config should have empty Project")
	}
}

func TestSnapshot_ScanDir(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0644)
	os.MkdirAll(filepath.Join(tmp, "sub"), 0755)
	os.WriteFile(filepath.Join(tmp, "sub", "b.txt"), []byte("b"), 0644)

	snap, err := SnapshotDir(tmp, nil)
	if err != nil {
		t.Fatalf("SnapshotDir() error = %v", err)
	}
	if len(snap) != 3 { // a.txt, sub/, sub/b.txt
		t.Errorf("snapshot size = %d, want 3", len(snap))
	}
}

func TestSnapshot_IgnorePatterns(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "keep.txt"), []byte("k"), 0644)
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	os.WriteFile(filepath.Join(tmp, ".git", "config"), []byte("g"), 0644)

	snap, _ := SnapshotDir(tmp, []string{".git/**"})
	for _, f := range snap {
		if filepath.Base(f.Path) == "config" {
			t.Error(".git files should be ignored")
		}
	}
}

func TestDiffSnapshots_Created(t *testing.T) {
	before := map[string]FileInfo{"existing.txt": {Size: 100}}
	after := map[string]FileInfo{
		"existing.txt": {Size: 100},
		"new.txt":      {Size: 200},
	}

	created, modified, deleted := DiffSnapshots(before, after)
	if len(created) != 1 || created[0].Path != "new.txt" {
		t.Errorf("created = %+v, want [new.txt]", created)
	}
	if len(modified) != 0 {
		t.Errorf("modified should be empty")
	}
	if len(deleted) != 0 {
		t.Errorf("deleted should be empty")
	}
}

func TestDiffSnapshots_Modified(t *testing.T) {
	before := map[string]FileInfo{"file.txt": {Size: 100}}
	after := map[string]FileInfo{"file.txt": {Size: 200}}

	_, modified, _ := DiffSnapshots(before, after)
	if len(modified) != 1 || modified[0].Path != "file.txt" {
		t.Errorf("modified count wrong, got %d", len(modified))
	}
}

func TestDiffSnapshots_Deleted(t *testing.T) {
	before := map[string]FileInfo{"gone.txt": {Size: 50}}
	after := map[string]FileInfo{}

	_, _, deleted := DiffSnapshots(before, after)
	if len(deleted) != 1 || deleted[0].Path != "gone.txt" {
		t.Errorf("deleted = %+v, want [gone.txt]", deleted)
	}
}

func TestClassifyArtifact_ByExtension(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"results/out.bam", "output"},
		{"data/input.fastq.gz", "input"},
		{"scripts/run.py", "script"},
		{"configs/conf.yaml", "config"},
		{"report.html", "report"},
		{"unknown.bin", "output"},
	}
	for _, tt := range tests {
		got := ClassifyArtifact(tt.path)
		if got != tt.want {
			t.Errorf("ClassifyArtifact(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestIsLargeFile_BySize(t *testing.T) {
	if !IsLargeFile(100_000_000) {
		t.Error("100MB file should be large")
	}
	if IsLargeFile(1000) {
		t.Error("1KB file should not be large")
	}
}
