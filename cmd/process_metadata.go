package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const ProcessMetadataFile = "process.json"

// ProcessMetadata identifies both the supervised root process and its process
// group. StartTimeTicks prevents a recycled PID from being treated as the
// original run.
type ProcessMetadata struct {
	Schema         int    `json:"schema"`
	PID            int    `json:"pid"`
	PGID           int    `json:"pgid"`
	StartTimeTicks uint64 `json:"start_time_ticks,omitempty"`
	CreatedAt      string `json:"created_at"`
}

type ProcessInspection struct {
	RootExists    bool
	IdentityMatch bool
	GroupAlive    bool
	ActualPGID    int
	ActualStart   uint64
}

func NewProcessMetadata(pid, pgid int) (ProcessMetadata, error) {
	if pid <= 0 || pgid <= 0 {
		return ProcessMetadata{}, fmt.Errorf("invalid process identity pid=%d pgid=%d", pid, pgid)
	}
	start, err := processStartTimeTicks(pid)
	if err != nil {
		return ProcessMetadata{}, fmt.Errorf("read process start time: %w", err)
	}
	return ProcessMetadata{
		Schema:         1,
		PID:            pid,
		PGID:           pgid,
		StartTimeTicks: start,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func WriteProcessMetadata(runDir string, metadata ProcessMetadata) error {
	if metadata.PID <= 0 || metadata.PGID <= 0 || metadata.Schema != 1 || metadata.StartTimeTicks == 0 {
		return errors.New("invalid process metadata")
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteFile(filepath.Join(runDir, ProcessMetadataFile), data, 0o644)
}

func ReadProcessMetadata(runDir string) (ProcessMetadata, error) {
	path := filepath.Join(runDir, ProcessMetadataFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return ProcessMetadata{}, err
	}
	var metadata ProcessMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return ProcessMetadata{}, fmt.Errorf("parse process metadata: %w", err)
	}
	if metadata.Schema != 1 || metadata.PID <= 0 || metadata.PGID <= 0 || metadata.StartTimeTicks == 0 {
		return ProcessMetadata{}, errors.New("invalid process metadata fields")
	}
	return metadata, nil
}

// ValidateStoredProcessIdentity ensures the runtime file describes the exact
// process identity committed to the database when the run entered running.
func ValidateStoredProcessIdentity(pid, pgid int, startTicks int64, metadata ProcessMetadata) error {
	if pid <= 0 || pgid <= 0 || startTicks <= 0 {
		return errors.New("database process identity is incomplete")
	}
	if metadata.PID != pid || metadata.PGID != pgid || metadata.StartTimeTicks != uint64(startTicks) {
		return fmt.Errorf("process identity mismatch: db=%d/%d/%d file=%d/%d/%d",
			pid, pgid, startTicks, metadata.PID, metadata.PGID, metadata.StartTimeTicks)
	}
	return nil
}

func InspectProcess(metadata ProcessMetadata) ProcessInspection {
	return inspectProcess(metadata)
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	ok = true
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}
