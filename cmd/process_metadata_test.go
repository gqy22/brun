package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessMetadataRoundTripAndRejectsLegacyPID(t *testing.T) {
	dir := t.TempDir()
	want := ProcessMetadata{
		Schema:         1,
		PID:            123,
		PGID:           120,
		StartTimeTicks: 456,
		CreatedAt:      "2026-07-15T01:02:03Z",
	}
	if err := WriteProcessMetadata(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadProcessMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("metadata = %+v, want %+v", got, want)
	}

	if err := os.WriteFile(filepath.Join(dir, ProcessMetadataFile), []byte("789\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadProcessMetadata(dir); err == nil {
		t.Fatal("integer-only process metadata should be rejected")
	}
}
