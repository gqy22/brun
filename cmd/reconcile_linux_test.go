//go:build linux

package cmd

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/biotools/brun/internal"
)

func TestReconcileRunMarksMissingProcessMetadataFailed(t *testing.T) {
	store, err := internal.NewStore(filepath.Join(t.TempDir(), "brun.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runDir := t.TempDir()
	run := &internal.Run{
		ID:        "stale-run",
		Status:    "running",
		StartedAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		RunDir:    runDir,
	}
	if err := store.CreateRun(run); err != nil {
		t.Fatal(err)
	}
	result, err := ReconcileRun(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Reason != "process_metadata_unavailable" {
		t.Fatalf("result = %+v", result)
	}
	updated, err := store.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "failed" || updated.EndedAt == "" {
		t.Fatalf("updated run = %+v", updated)
	}
	if _, err := os.Stat(filepath.Join(runDir, "metadata.yaml")); err != nil {
		t.Fatalf("metadata.yaml missing: %v", err)
	}
}

func TestReconcileRunKeepsMatchingLiveProcess(t *testing.T) {
	store, err := internal.NewStore(filepath.Join(t.TempDir(), "brun.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runDir := t.TempDir()
	pgid, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := NewProcessMetadata(os.Getpid(), pgid)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteProcessMetadata(runDir, metadata); err != nil {
		t.Fatal(err)
	}
	run := &internal.Run{
		ID:                "live-run",
		Status:            "running",
		StartedAt:         time.Now().UTC().Format(time.RFC3339),
		RunDir:            runDir,
		ProcessPID:        metadata.PID,
		ProcessPGID:       metadata.PGID,
		ProcessStartTicks: int64(metadata.StartTimeTicks),
	}
	if err := store.CreateRun(run); err != nil {
		t.Fatal(err)
	}
	result, err := ReconcileRun(store, run)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed {
		t.Fatalf("live run was reconciled: %+v", result)
	}
}
