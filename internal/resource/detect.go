package resource

import "fmt"

const (
	BackendProc        = "proc"
	BackendCgroupV2    = "cgroup_v2"
	BackendUnsupported = "unsupported"
)

type Environment struct {
	Unified     bool
	Mountpoint  string
	CurrentPath string
	FullPath    string
	Delegated   bool
	Controllers []string
	Reason      string
}

type Decision struct {
	Requested string
	Backend   string
	Fallback  string
	Detail    string
	Env       Environment
}

func Decide(mode Mode) (Decision, error) {
	return Resolve(mode, "")
}

// Resolve selects a resource backend and, on Linux, first tries to acquire a
// delegated transient systemd user scope when cgroup v2 is available but the
// current process has not already been delegated one.
func Resolve(mode Mode, runID string) (Decision, error) {
	return resolve(mode, runID, Detect(), ProcSupported(), AcquireDelegation)
}

func resolve(mode Mode, runID string, env Environment, procSupported bool, acquire func(string) (Environment, error)) (Decision, error) {
	if mode != ModeProc && env.Unified && !env.Delegated && runID != "" {
		delegated, err := acquire(runID)
		if err == nil {
			env = delegated
		} else if mode == ModeCgroup {
			return Decision{}, fmt.Errorf("cgroup backend unavailable: automatic systemd delegation failed: %w", err)
		} else {
			decision, decideErr := decide(mode, env, procSupported)
			if decideErr != nil {
				return Decision{}, decideErr
			}
			decision.Fallback = "systemd_scope_unavailable"
			decision.Detail = err.Error()
			return decision, nil
		}
	}
	return decide(mode, env, procSupported)
}

func decide(mode Mode, env Environment, procSupported bool) (Decision, error) {
	decision := Decision{Requested: string(mode), Env: env}
	switch mode {
	case ModeProc:
		if procSupported {
			decision.Backend = BackendProc
		} else {
			decision.Backend = BackendUnsupported
			decision.Fallback = "not_linux"
		}
		return decision, nil
	case ModeCgroup:
		if !env.Unified || !env.Delegated {
			return Decision{}, fmt.Errorf("cgroup backend unavailable: %s", env.Reason)
		}
		decision.Backend = BackendCgroupV2
		return decision, nil
	case ModeAuto:
		if env.Unified && env.Delegated {
			decision.Backend = BackendCgroupV2
			return decision, nil
		}
		if procSupported {
			decision.Backend = BackendProc
			decision.Fallback = env.Reason
		} else {
			decision.Backend = BackendUnsupported
			decision.Fallback = "not_linux"
		}
		return decision, nil
	default:
		return Decision{}, fmt.Errorf("unsupported resource mode %q", mode)
	}
}
