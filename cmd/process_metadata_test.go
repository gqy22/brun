package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessMetadataRoundTripAndLegacyCompatibility(t *testing.T) {
	dir := t.TempDir()
	want := ProcessMetadata{
		Version:        1,
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
	legacy, err := ReadProcessMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.PID != 789 || legacy.PGID != 789 || !legacy.Legacy {
		t.Fatalf("legacy metadata = %+v", legacy)
	}
}
