package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const DiagnosticsFileName = "diagnostics.jsonl"

type DiagnosticEvent struct {
	Level     string `json:"level"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Detail    string `json:"detail,omitempty"`
	Source    string `json:"source,omitempty"`
	CreatedAt string `json:"created_at"`
}

type DiagnosticSummary struct {
	InfoCount    int    `json:"info_count"`
	WarningCount int    `json:"warning_count"`
	ErrorCount   int    `json:"error_count"`
	LastLevel    string `json:"last_level,omitempty"`
	LastCode     string `json:"last_code,omitempty"`
	LastMessage  string `json:"last_message,omitempty"`
	LastAt       string `json:"last_at,omitempty"`
}

type DiagnosticWriter struct {
	path        string
	afterRecord func()
}

func NewDiagnosticWriter(runDir string) *DiagnosticWriter {
	return &DiagnosticWriter{path: filepath.Join(runDir, DiagnosticsFileName)}
}

func (w *DiagnosticWriter) SetAfterRecord(fn func()) {
	if w != nil {
		w.afterRecord = fn
	}
}

func (w *DiagnosticWriter) Info(code, message, detail string) {
	w.record("info", code, message, detail, "cli")
}

func (w *DiagnosticWriter) Warning(code, message, detail string) {
	w.record("warning", code, message, detail, "cli")
}

func (w *DiagnosticWriter) Error(code, message, detail string) {
	w.record("error", code, message, detail, "cli")
}

func (w *DiagnosticWriter) record(level, code, message, detail, source string) {
	if w == nil || w.path == "" {
		return
	}
	event := DiagnosticEvent{
		Level:     level,
		Code:      code,
		Message:   message,
		Detail:    detail,
		Source:    source,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		Log().Warn("diagnostic_write_failed", "path", w.path, "error", err.Error())
		return
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(event); err != nil {
		Log().Warn("diagnostic_encode_failed", "path", w.path, "error", err.Error())
		return
	}
	if w.afterRecord != nil {
		w.afterRecord()
	}
}

func ReadDiagnostics(runDir string) ([]DiagnosticEvent, error) {
	f, err := os.Open(filepath.Join(runDir, DiagnosticsFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []DiagnosticEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event DiagnosticEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("read diagnostics: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func DiagnosticSummaryFromRun(r *Run) DiagnosticSummary {
	if r == nil {
		return DiagnosticSummary{}
	}
	return DiagnosticSummary{
		WarningCount: r.DiagWarningCount,
		ErrorCount:   r.DiagErrorCount,
		LastCode:     r.DiagLastCode,
		LastAt:       r.DiagLastAt,
	}
}

func ReadDiagnosticSummary(runDir string) (DiagnosticSummary, error) {
	events, err := ReadDiagnostics(runDir)
	if err != nil {
		return DiagnosticSummary{}, err
	}
	return SummarizeDiagnostics(events), nil
}

func SummarizeDiagnostics(events []DiagnosticEvent) DiagnosticSummary {
	var summary DiagnosticSummary
	for _, event := range events {
		switch event.Level {
		case "error":
			summary.ErrorCount++
		case "warning":
			summary.WarningCount++
		case "info":
			summary.InfoCount++
		}
		summary.LastLevel = event.Level
		summary.LastCode = event.Code
		summary.LastMessage = event.Message
		summary.LastAt = event.CreatedAt
	}
	return summary
}

func DiagnosticWarnings(events []DiagnosticEvent) []DiagnosticEvent {
	var warnings []DiagnosticEvent
	for _, event := range events {
		if event.Level == "warning" || event.Level == "error" {
			warnings = append(warnings, event)
		}
	}
	return warnings
}
