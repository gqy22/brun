//go:build linux

package resource

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseUnifiedPath(t *testing.T) {
	got, err := parseUnifiedPath([]byte("0::/user.slice/test.scope\n"))
	if err != nil || got != "/user.slice/test.scope" {
		t.Fatalf("parseUnifiedPath() = %q, %v", got, err)
	}
	for _, input := range []string{"", "1:name=/legacy", "0::/gone (deleted)"} {
		if _, err := parseUnifiedPath([]byte(input)); err == nil {
			t.Fatalf("parseUnifiedPath(%q) succeeded", input)
		}
	}
}

func TestSafeCgroupPath(t *testing.T) {
	root := t.TempDir()
	got, err := safeCgroupPath(root, "/user.slice/test.scope")
	if err != nil || got != filepath.Join(root, "user.slice/test.scope") {
		t.Fatalf("safeCgroupPath() = %q, %v", got, err)
	}
	if _, err := safeCgroupPath(root, "relative"); err == nil {
		t.Fatal("relative path accepted")
	}
}

func TestDelegatedByProbe(t *testing.T) {
	dir := t.TempDir()
	if !delegatedByProbe(dir) {
		t.Fatal("writable directory not detected")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("probe leaked entries: %v, %v", entries, err)
	}
}
