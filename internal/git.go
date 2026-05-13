package internal

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func GenerateRunID() string {
	now := time.Now()
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("%s-%s-%x", now.Format("20060102"), now.Format("150405"), b)
}

func HomeDir() string {
	if d := os.Getenv("BRUN_HOME"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".bio-runner")
}

func RunDir(runID string) string {
	if len(runID) < 8 {
		return filepath.Join(HomeDir(), "runs", runID)
	}
	return filepath.Join(HomeDir(), "runs", runID[0:4], runID[4:6], runID[6:8], runID)
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

type ProjectOption func(*projectOpts)

type projectOpts struct {
	cliProject string
}

func WithCLIProject(p string) ProjectOption {
	return func(o *projectOpts) { o.cliProject = p }
}

func DetectProject(cwd string, opts ...ProjectOption) (string, string) {
	var po projectOpts
	for _, o := range opts {
		o(&po)
	}
	if po.cliProject != "" {
		return po.cliProject, cwd
	}
	yamlPath := filepath.Join(cwd, "brun.yaml")
	if data, err := os.ReadFile(yamlPath); err == nil {
		if cfg, err := ParseConfig(data); err == nil && cfg.Project != "" {
			return cfg.Project, cwd
		}
	}
	return filepath.Base(cwd), cwd
}

type GitInfo struct {
	Repo   string
	Branch string
	Commit string
	Dirty  bool
}

func CollectGitInfo(dir string) GitInfo {
	info := GitInfo{}
	repo := gitOutput(dir, "rev-parse", "--show-toplevel")
	if repo == "" {
		return info
	}
	info.Repo = repo
	info.Branch = gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
	info.Commit = gitOutput(dir, "rev-parse", "HEAD")
	info.Dirty = gitHasChanges(dir)
	return info
}

func gitHasChanges(dir string) bool {
	// 检查已修改文件
	if c := exec.Command("git", "-C", dir, "diff", "--quiet"); c.Run() != nil {
		return true
	}
	// 检查未跟踪文件
	out, _ := exec.Command("git", "-C", dir, "ls-files", "--others", "--exclude-standard").Output()
	return len(out) > 0
}

func gitOutput(dir string, args ...string) string {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
