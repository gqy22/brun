package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/biotools/brun/cmd"
	"github.com/biotools/brun/internal"
	resourcepkg "github.com/biotools/brun/internal/resource"
)

const (
	launchFDEnv   = "BRUN_LAUNCH_FD"
	launchTimeout = 30 * time.Second
)

type launchMessage struct {
	Status  string `json:"status"`
	RunID   string `json:"run_id"`
	Backend string `json:"backend,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

type launchNotifier struct {
	file *os.File
}

func launchNotifierFromEnv() *launchNotifier {
	raw := os.Getenv(launchFDEnv)
	if raw == "" {
		return nil
	}
	fd, err := strconv.Atoi(raw)
	if err != nil || fd < 3 {
		return nil
	}
	return &launchNotifier{file: os.NewFile(uintptr(fd), "brun-launch")}
}

func (n *launchNotifier) send(message launchMessage) error {
	if n == nil || n.file == nil {
		return nil
	}
	err := json.NewEncoder(n.file).Encode(message)
	closeErr := n.file.Close()
	n.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

func (n *launchNotifier) ready(runID, backend string) error {
	return n.send(launchMessage{Status: "ready", RunID: runID, Backend: backend})
}

func (n *launchNotifier) failed(runID string, err error) {
	if n == nil || n.file == nil {
		return
	}
	message := launchMessage{Status: "failed", RunID: runID, Code: "launch_failed", Message: err.Error()}
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		message.Code = cliErr.Code
		message.Message = cliErr.Error()
		message.Hint = cliErr.Hint
	}
	_ = n.send(message)
}

func waitLaunch(reader io.Reader, timeout time.Duration) (launchMessage, error) {
	result := make(chan struct {
		message launchMessage
		err     error
	}, 1)
	go func() {
		var message launchMessage
		err := json.NewDecoder(reader).Decode(&message)
		result <- struct {
			message launchMessage
			err     error
		}{message: message, err: err}
	}()
	select {
	case decoded := <-result:
		if decoded.err != nil {
			return launchMessage{}, fmt.Errorf("读取后台启动状态: %w", decoded.err)
		}
		if decoded.message.Status != "ready" && decoded.message.Status != "failed" {
			return launchMessage{}, fmt.Errorf("无效后台启动状态 %q", decoded.message.Status)
		}
		return decoded.message, nil
	case <-time.After(timeout):
		return launchMessage{}, fmt.Errorf("等待后台启动状态超时 (%s)", timeout)
	}
}

func recordDetachedLaunchFailure(runID, runDir, cwd, command, name, project string, resourceMode resourcepkg.Mode, cause error) {
	now := time.Now().UTC().Format(time.RFC3339)
	code := "launch_failed"
	message := cause.Error()
	var cliErr *CLIError
	if errors.As(cause, &cliErr) {
		code = cliErr.Code
		message = cliErr.Error()
	}
	diagnostics := internal.NewDiagnosticWriter(runDir)
	diagnostics.Error(code, message, "payload 未启动")
	summary, _ := internal.ReadDiagnosticSummary(runDir)
	host, hostStatus := hostname()
	user, userStatus := username()
	run := &internal.Run{
		ID: runID, Name: name, Project: project, CWD: cwd, Command: command,
		Status: "failed", ExitCode: 1, StartedAt: now, EndedAt: now, RunDir: runDir,
		Hostname: host, HostnameStatus: hostStatus, Username: user, UsernameStatus: userStatus,
		ResourceRequested: string(resourceMode), ResourceBackend: resourcepkg.BackendUnsupported,
		ResourceFallback: code, ResourceStatus: "unavailable", ResourceSupported: false,
		TerminationReason: "launch_failed", DiagInfoCount: summary.InfoCount,
		DiagWarningCount: summary.WarningCount, DiagErrorCount: summary.ErrorCount,
		DiagLastCode: summary.LastCode, DiagLastAt: summary.LastAt,
	}
	store, err := openStore()
	if err == nil {
		if existing, getErr := store.GetRun(runID); getErr == nil {
			_, _ = store.FinalizeRun(existing.ID, "failed", 1, now, 0, "launch_failed", "", false)
			_ = store.UpdateRunDiagnostics(existing.ID, summary)
			existing.Status = "failed"
			existing.ExitCode = 1
			existing.EndedAt = now
			existing.TerminationReason = "launch_failed"
			existing.DiagInfoCount = summary.InfoCount
			existing.DiagWarningCount = summary.WarningCount
			existing.DiagErrorCount = summary.ErrorCount
			existing.DiagLastCode = summary.LastCode
			existing.DiagLastAt = summary.LastAt
			run = existing
		} else if createErr := store.CreateRun(run); createErr != nil {
			internal.Log().Warn("launch_failure_store_failed", "run_id", runID, "error", createErr.Error())
		}
		_ = store.Close()
	} else {
		internal.Log().Warn("launch_failure_store_unavailable", "run_id", runID, "error", err.Error())
	}
	if err := os.WriteFile(filepath.Join(runDir, "metadata.yaml"), []byte(cmd.BuildMetadataYAML(run)), 0o644); err != nil {
		internal.Log().Warn("launch_failure_metadata_failed", "run_id", runID, "error", err.Error())
	}
}
