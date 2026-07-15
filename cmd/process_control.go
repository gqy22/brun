package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const TerminationRecordFile = "termination.json"

type TerminationRecord struct {
	Schema    int    `json:"schema"`
	Reason    string `json:"reason"`
	Signal    string `json:"signal"`
	Escalated bool   `json:"escalated"`
	CreatedAt string `json:"created_at"`
}

type StopResult struct {
	OK               bool   `json:"ok"`
	PID              int    `json:"pid,omitempty"`
	PGID             int    `json:"pgid,omitempty"`
	Msg              string `json:"msg,omitempty"`
	Signal           string `json:"signal,omitempty"`
	Escalated        bool   `json:"escalated,omitempty"`
	GroupGone        bool   `json:"group_gone,omitempty"`
	AlreadyDead      bool   `json:"already_dead,omitempty"`
	IdentityMismatch bool   `json:"identity_mismatch,omitempty"`
}

func WriteTerminationRecord(runDir string, record TerminationRecord) error {
	if runDir == "" {
		return nil
	}
	if record.Schema == 0 {
		record.Schema = 1
	}
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(runDir, TerminationRecordFile), append(data, '\n'), 0o644)
}

func ReadTerminationRecord(runDir string) (TerminationRecord, error) {
	data, err := os.ReadFile(filepath.Join(runDir, TerminationRecordFile))
	if err != nil {
		return TerminationRecord{}, err
	}
	var record TerminationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return TerminationRecord{}, err
	}
	return record, nil
}
