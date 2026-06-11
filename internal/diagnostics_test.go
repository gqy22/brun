package internal

import "testing"

func TestDiagnosticWriterRecordsJSONL(t *testing.T) {
	runDir := fastTempDir(t)
	writer := NewDiagnosticWriter(runDir)

	writer.Info("cwd_inferred", "已推断运行目录", "/tmp/project")
	writer.Warning("metadata_write_failed", "metadata.yaml 写入失败", "permission denied")

	events, err := ReadDiagnostics(runDir)
	if err != nil {
		t.Fatalf("ReadDiagnostics() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Code != "cwd_inferred" || events[0].Level != "info" {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].Code != "metadata_write_failed" || events[1].Level != "warning" {
		t.Fatalf("unexpected second event: %+v", events[1])
	}

	warnings := DiagnosticWarnings(events)
	if len(warnings) != 1 || warnings[0].Code != "metadata_write_failed" {
		t.Fatalf("warnings = %+v, want metadata_write_failed only", warnings)
	}
}

func TestReadDiagnosticsMissingFileReturnsEmpty(t *testing.T) {
	events, err := ReadDiagnostics(fastTempDir(t))
	if err != nil {
		t.Fatalf("ReadDiagnostics() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("len(events) = %d, want 0", len(events))
	}
}

func TestSummarizeDiagnostics(t *testing.T) {
	events := []DiagnosticEvent{
		{Level: "info", Code: "cwd_inferred", Message: "已推断运行目录", CreatedAt: "2026-06-05T01:00:00Z"},
		{Level: "warning", Code: "metadata_write_failed", Message: "metadata.yaml 写入失败", CreatedAt: "2026-06-05T01:01:00Z"},
		{Level: "error", Code: "diagnostic_failed", Message: "诊断失败", CreatedAt: "2026-06-05T01:02:00Z"},
	}

	summary := SummarizeDiagnostics(events)
	if summary.InfoCount != 1 || summary.WarningCount != 1 || summary.ErrorCount != 1 {
		t.Fatalf("summary counts = %+v, want 1/1/1", summary)
	}
	if summary.LastCode != "diagnostic_failed" || summary.LastAt != "2026-06-05T01:02:00Z" {
		t.Fatalf("last event summary = %+v", summary)
	}
}

func TestDiagnosticSummaryFromRunIncludesInfoCount(t *testing.T) {
	summary := DiagnosticSummaryFromRun(&Run{
		DiagInfoCount:    1,
		DiagWarningCount: 2,
		DiagErrorCount:   3,
		DiagLastCode:     "script_snapshot_missing",
		DiagLastAt:       "2026-06-05T01:02:00Z",
	})
	if summary.InfoCount != 1 || summary.WarningCount != 2 || summary.ErrorCount != 3 {
		t.Fatalf("summary counts = %+v, want 1/2/3", summary)
	}
	if summary.LastCode != "script_snapshot_missing" || summary.LastAt != "2026-06-05T01:02:00Z" {
		t.Fatalf("last event summary = %+v", summary)
	}
}

func TestDiagnosticWriterInvokesCounterAfterJSONL(t *testing.T) {
	runDir := fastTempDir(t)
	writer := NewDiagnosticWriter(runDir)

	var calls []string
	writer.SetCounter(func(level, code, lastAt string) {
		calls = append(calls, level+":"+code+"@"+lastAt)
	})

	writer.Info("cwd_inferred", "已推断运行目录", "/tmp/project")
	writer.Warning("metadata_write_failed", "metadata.yaml 写入失败", "permission denied")
	writer.Error("boom", "crash", "")

	if len(calls) != 3 {
		t.Fatalf("counter called %d times, want 3", len(calls))
	}
	wantPrefixes := []string{"info:cwd_inferred@", "warning:metadata_write_failed@", "error:boom@"}
	for i, want := range wantPrefixes {
		if len(calls[i]) < len(want) || calls[i][:len(want)] != want {
			t.Errorf("call %d = %q, want prefix %q", i, calls[i], want)
		}
	}

	events, err := ReadDiagnostics(runDir)
	if err != nil {
		t.Fatalf("ReadDiagnostics() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("jsonl events = %d, want 3", len(events))
	}
}
