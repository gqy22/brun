package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

func TestGenerateRunID(t *testing.T) {
	id := GenerateRunID()

	// 格式: YYYYMMDD-HHMMSS-xxxxxx (6位hex)
	pattern := `^\d{8}-\d{6}-[a-f0-9]{6}$`
	matched, _ := regexp.MatchString(pattern, id)
	if !matched {
		t.Errorf("GenerateRunID() = %q, want match %s", id, pattern)
	}
}

func TestGenerateRunID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateRunID()
		if ids[id] {
			t.Errorf("duplicate run_id: %s", id)
		}
		ids[id] = true
	}
}

func TestHomeDir_Default(t *testing.T) {
	os.Unsetenv("BRUN_HOME")
	dir := HomeDir()
	expected := filepath.Join(os.Getenv("HOME"), ".bio-runner")
	if dir != expected {
		t.Errorf("HomeDir() = %q, want %q", dir, expected)
	}
}

func TestHomeDir_Custom(t *testing.T) {
	t.Setenv("BRUN_HOME", "/tmp/brun-test")
	dir := HomeDir()
	if dir != "/tmp/brun-test" {
		t.Errorf("HomeDir() = %q, want %q", dir, "/tmp/brun-test")
	}
}

func TestRunDir(t *testing.T) {
	t.Setenv("BRUN_HOME", "/tmp/brun-test")
	runID := "20260513-153012-a8f3c2"

	dir := RunDir(runID)
	expected := "/tmp/brun-test/runs/2026/05/13/20260513-153012-a8f3c2"
	if dir != expected {
		t.Errorf("RunDir() = %q, want %q", dir, expected)
	}
}

func TestEnsureDir(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "a", "b", "c")

	err := EnsureDir(dir)
	if err != nil {
		t.Fatalf("EnsureDir() error = %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Errorf("EnsureDir() did not create directory")
	}
}

func TestDetectProject_FromYAML(t *testing.T) {
	tmp := t.TempDir()
	yamlPath := filepath.Join(tmp, "brun.yaml")
	os.WriteFile(yamlPath, []byte("project: my-rnaseq\n"), 0644)

	project, root := DetectProject(tmp)
	if project != "my-rnaseq" {
		t.Errorf("DetectProject() project = %q, want %q", project, "my-rnaseq")
	}
	if root != tmp {
		t.Errorf("DetectProject() root = %q, want %q", root, tmp)
	}
}

func TestDetectProject_FromDirName(t *testing.T) {
	tmp := t.TempDir()
	// 没有 brun.yaml，没有 git，应该返回目录名
	project, _ := DetectProject(tmp)
	base := filepath.Base(tmp)
	if project != base {
		t.Errorf("DetectProject() project = %q, want %q (dir name)", project, base)
	}
}

func TestDetectProject_Priority_CLIOverYAML(t *testing.T) {
	tmp := t.TempDir()
	yamlPath := filepath.Join(tmp, "brun.yaml")
	os.WriteFile(yamlPath, []byte("project: yaml-project\n"), 0644)

	project, _ := DetectProject(tmp, WithCLIProject("cli-project"))
	if project != "cli-project" {
		t.Errorf("CLI project should override YAML: got %q, want %q", project, "cli-project")
	}
}

func TestCollectGitInfo_InGitRepo(t *testing.T) {
	tmp := t.TempDir()
	// 初始化一个 git repo
	runCmd(t, tmp, "git", "init")
	runCmd(t, tmp, "git", "config", "user.email", "test@test.com")
	runCmd(t, tmp, "git", "config", "user.name", "test")
	runCmd(t, tmp, "git", "commit", "--allow-empty", "-m", "init")

	info := CollectGitInfo(tmp)
	if info.Repo == "" {
		t.Error("CollectGitInfo().Repo should not be empty in git repo")
	}
	if info.Commit == "" {
		t.Error("CollectGitInfo().Commit should not be empty after commit")
	}
	if info.Dirty {
		t.Error("Clean repo should have Dirty=false")
	}
}

func TestCollectGitInfo_DirtyRepo(t *testing.T) {
	tmp := t.TempDir()
	runCmd(t, tmp, "git", "init")
	runCmd(t, tmp, "git", "config", "user.email", "test@test.com")
	runCmd(t, tmp, "git", "config", "user.name", "test")
	runCmd(t, tmp, "git", "commit", "--allow-empty", "-m", "init")
	os.WriteFile(filepath.Join(tmp, "dirty.txt"), []byte("dirty"), 0644)

	info := CollectGitInfo(tmp)
	if !info.Dirty {
		t.Error("Repo with uncommitted file should have Dirty=true")
	}
}

func TestCollectGitInfo_NotGitRepo(t *testing.T) {
	tmp := t.TempDir()
	info := CollectGitInfo(tmp)
	if info.Repo != "" || info.Commit != "" {
		t.Errorf("Non-git repo should return empty info, got %+v", info)
	}
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	if err := c.Run(); err != nil {
		t.Fatalf("run %s %v: %v", name, args, err)
	}
}
