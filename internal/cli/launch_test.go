package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/biotools/brun/internal"
	resourcepkg "github.com/biotools/brun/internal/resource"
)

func TestLaunchHandshakeReady(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	notifier := &launchNotifier{file: writer}
	go func() { _ = notifier.ready("run-12345678", resourcepkg.BackendCgroupV2) }()
	message, err := waitLaunch(reader, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if message.Status != "ready" || message.RunID != "run-12345678" || message.Backend != resourcepkg.BackendCgroupV2 {
		t.Fatalf("message = %+v", message)
	}
}

func TestLaunchHandshakeFailurePreservesCLIError(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	notifier := &launchNotifier{file: writer}
	go notifier.failed("run-12345678", cliError("resource_backend_unavailable", "no delegation", "check systemd", nil))
	message, err := waitLaunch(reader, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if message.Status != "failed" || message.Code != "resource_backend_unavailable" || message.Hint != "check systemd" {
		t.Fatalf("message = %+v", message)
	}
}

func TestRecordDetachedLaunchFailure(t *testing.T) {
	home := fastTempDir(t)
	t.Setenv("BRUN_HOME", home)
	runID := "20260716-120000-abcdef"
	runDir := internal.RunDir(runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recordDetachedLaunchFailure(runID, runDir, "/tmp", "true", "", "proj", resourcepkg.ModeCgroup, errors.New("boom"))
	store, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run, err := store.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" || run.TerminationReason != "launch_failed" || run.DiagErrorCount != 1 {
		t.Fatalf("run = %+v", run)
	}
	if _, err := os.Stat(filepath.Join(runDir, "metadata.yaml")); err != nil {
		t.Fatal(err)
	}
}
