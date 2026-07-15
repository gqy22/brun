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
	Env       Environment
}

func Decide(mode Mode) (Decision, error) {
	return decide(mode, Detect(), ProcSupported())
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
