//go:build linux

package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	systemddbus "github.com/coreos/go-systemd/v22/dbus"
	godbus "github.com/godbus/dbus/v5"
)

const delegationTimeout = 3 * time.Second

type transientScopeStarter interface {
	StartTransientUnitContext(context.Context, string, string, []systemddbus.Property, chan<- string) (int, error)
}

// AcquireDelegation asks the per-user systemd manager to move brun into a
// transient scope with Delegate=yes. The scope remains alive while brun is
// alive and is collected automatically after the process exits.
func AcquireDelegation(runID string) (Environment, error) {
	env := Detect()
	if env.Delegated {
		exclusive, err := delegatedRootExclusive(env.FullPath, os.Getpid())
		if err == nil && exclusive {
			return env, nil
		}
	}
	if !env.Unified {
		return env, fmt.Errorf("cgroup v2 unavailable: %s", env.Reason)
	}
	if !validRunID(runID) {
		return env, fmt.Errorf("invalid run id for systemd scope: %q", runID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), delegationTimeout)
	defer cancel()
	conn, err := systemddbus.NewUserConnectionContext(ctx)
	if err != nil {
		return env, fmt.Errorf("connect to systemd user manager: %w", err)
	}
	defer conn.Close()

	unit := "brun-" + runID + ".scope"
	if err := startDelegatedScope(ctx, conn, unit, os.Getpid()); err != nil {
		return env, err
	}
	previousPath := ""
	if env.Delegated {
		previousPath = env.CurrentPath
	}
	return waitForDelegation(ctx, previousPath)
}

func delegatedRootExclusive(root string, pid int) (bool, error) {
	if root == "" || pid <= 0 {
		return false, fmt.Errorf("invalid delegated root %q or pid %d", root, pid)
	}
	data, err := os.ReadFile(filepath.Join(root, "cgroup.procs"))
	if err != nil {
		return false, err
	}
	fields := strings.Fields(string(data))
	if len(fields) != 1 {
		return false, nil
	}
	current, err := strconv.Atoi(fields[0])
	if err != nil {
		return false, err
	}
	return current == pid, nil
}

func startDelegatedScope(ctx context.Context, manager transientScopeStarter, unit string, pid int) error {
	if pid <= 0 || !strings.HasPrefix(unit, "brun-") || !strings.HasSuffix(unit, ".scope") {
		return fmt.Errorf("invalid transient scope unit=%q pid=%d", unit, pid)
	}
	properties := []systemddbus.Property{
		systemddbus.PropDescription("brun resource scope"),
		systemddbus.PropPids(uint32(pid)),
		{Name: "Delegate", Value: godbus.MakeVariant(true)},
		{Name: "CollectMode", Value: godbus.MakeVariant("inactive-or-failed")},
	}
	result := make(chan string, 1)
	if _, err := manager.StartTransientUnitContext(ctx, unit, "fail", properties, result); err != nil {
		return fmt.Errorf("start systemd user scope %s: %w", unit, err)
	}
	select {
	case state := <-result:
		if state != "done" {
			return fmt.Errorf("start systemd user scope %s: job result %s", unit, state)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("start systemd user scope %s: %w", unit, ctx.Err())
	}
}

func waitForDelegation(ctx context.Context, previousPath string) (Environment, error) {
	var env Environment
	for {
		env = Detect()
		if env.Delegated && (previousPath == "" || env.CurrentPath != previousPath) {
			return env, nil
		}
		select {
		case <-ctx.Done():
			return env, fmt.Errorf("wait for delegated cgroup: %w", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}
