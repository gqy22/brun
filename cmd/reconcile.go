package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/biotools/brun/internal"
)

const processMetadataStartupGrace = 2 * time.Minute

type ReconcileResult struct {
	RunID   string
	Changed bool
	Reason  string
}

func ReconcileRunningRuns(store *internal.Store, limit int) ([]ReconcileResult, error) {
	runs, err := store.ListRuns(limit, "", "running", "", "", "", "", false, "", "")
	if err != nil {
		return nil, err
	}
	results := make([]ReconcileResult, 0, len(runs))
	for _, run := range runs {
		result, err := ReconcileRun(store, run)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func ReconcileRun(store *internal.Store, run *internal.Run) (ReconcileResult, error) {
	if run == nil {
		return ReconcileResult{}, nil
	}
	result := ReconcileResult{RunID: run.ID}
	if run.Status != "running" {
		return result, nil
	}
	metadata, err := ReadProcessMetadata(run.RunDir)
	if err != nil {
		if runAge(run) < processMetadataStartupGrace && os.IsNotExist(err) {
			return result, nil
		}
		result.Reason = "process_metadata_unavailable"
		result.Changed = true
		return result, markReconciledFailed(store, run, result.Reason, err.Error())
	}
	inspection := InspectProcess(metadata)
	if inspection.RootExists && !inspection.IdentityMatch {
		result.Reason = "process_identity_mismatch"
		result.Changed = true
		return result, markReconciledFailed(store, run, result.Reason,
			fmt.Sprintf("pid=%d expected_pgid=%d actual_pgid=%d", metadata.PID, metadata.PGID, inspection.ActualPGID))
	}
	if inspection.GroupAlive {
		return result, nil
	}
	result.Reason = "process_group_gone"
	result.Changed = true
	return result, markReconciledFailed(store, run, result.Reason, fmt.Sprintf("pgid=%d", metadata.PGID))
}

func FinalizeRunState(store *internal.Store, run *internal.Run, status string, exitCode int) error {
	endedAt := time.Now().UTC()
	duration := run.DurationMs
	if started, err := time.Parse(time.RFC3339, run.StartedAt); err == nil {
		duration = endedAt.Sub(started).Milliseconds()
		if duration < 0 {
			duration = 0
		}
	}
	if err := store.UpdateRunStatus(run.ID, status, exitCode, endedAt.Format(time.RFC3339), duration); err != nil {
		return err
	}
	run.Status = status
	run.ExitCode = exitCode
	run.EndedAt = endedAt.Format(time.RFC3339)
	run.DurationMs = duration
	latest, err := store.GetRun(run.ID)
	if err != nil {
		return err
	}
	return writeRunMetadata(latest)
}

func CompleteUserStop(store *internal.Store, run *internal.Run, result StopResult) error {
	latest := run
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, err := store.GetRun(run.ID)
		if err != nil {
			return err
		}
		latest = current
		if current.Status != "running" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	writer := internal.NewDiagnosticWriter(run.RunDir)
	writer.Info("run_cancelled", "任务被用户终止",
		fmt.Sprintf("signal=%s escalated=%t group_gone=%t", result.Signal, result.Escalated, result.GroupGone))
	if err := store.IncrementRunDiagnostic(run.ID, "info", "run_cancelled", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	if latest.Status != "cancelled" {
		return FinalizeRunState(store, latest, "cancelled", 130)
	}
	refreshed, err := store.GetRun(run.ID)
	if err != nil {
		return err
	}
	return writeRunMetadata(refreshed)
}

func writeRunMetadata(run *internal.Run) error {
	return atomicWriteFile(filepath.Join(run.RunDir, "metadata.yaml"), []byte(BuildMetadataYAML(run)), 0o644)
}

func markReconciledFailed(store *internal.Store, run *internal.Run, reason, detail string) error {
	writer := internal.NewDiagnosticWriter(run.RunDir)
	writer.Warning("run_reconciled_failed", "检测到失联的 running 任务", reason+": "+detail)
	if err := store.IncrementRunDiagnostic(run.ID, "warning", "run_reconciled_failed", time.Now().UTC().Format(time.RFC3339)); err != nil {
		internal.Log().Warn("reconcile_diagnostic_update_failed", "run_id", run.ID, "error", err.Error())
	}
	return FinalizeRunState(store, run, "failed", -1)
}

func runAge(run *internal.Run) time.Duration {
	started, err := time.Parse(time.RFC3339, run.StartedAt)
	if err != nil {
		return 0
	}
	return time.Since(started)
}
