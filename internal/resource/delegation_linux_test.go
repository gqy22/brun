//go:build linux

package resource

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	systemddbus "github.com/coreos/go-systemd/v22/dbus"
)

func TestDelegatedRootExclusive(t *testing.T) {
	root := t.TempDir()
	pid := os.Getpid()
	path := filepath.Join(root, "cgroup.procs")
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exclusive, err := delegatedRootExclusive(root, pid)
	if err != nil || !exclusive {
		t.Fatalf("exclusive = %t, err = %v", exclusive, err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exclusive, err = delegatedRootExclusive(root, pid)
	if err != nil || exclusive {
		t.Fatalf("shared exclusive = %t, err = %v", exclusive, err)
	}
}

type fakeTransientScopeStarter struct {
	unit       string
	mode       string
	properties []systemddbus.Property
	result     string
	err        error
}

func (f *fakeTransientScopeStarter) StartTransientUnitContext(_ context.Context, unit, mode string, properties []systemddbus.Property, result chan<- string) (int, error) {
	f.unit = unit
	f.mode = mode
	f.properties = properties
	if f.err != nil {
		return 0, f.err
	}
	result <- f.result
	return 1, nil
}

func TestStartDelegatedScope(t *testing.T) {
	fake := &fakeTransientScopeStarter{result: "done"}
	if err := startDelegatedScope(context.Background(), fake, "brun-20260716-120000-abcdef.scope", 1234); err != nil {
		t.Fatal(err)
	}
	if fake.unit != "brun-20260716-120000-abcdef.scope" || fake.mode != "fail" {
		t.Fatalf("unexpected transient request: unit=%q mode=%q", fake.unit, fake.mode)
	}
	properties := make(map[string]any, len(fake.properties))
	for _, property := range fake.properties {
		properties[property.Name] = property.Value.Value()
	}
	if properties["Delegate"] != true || properties["CollectMode"] != "inactive-or-failed" {
		t.Fatalf("delegation properties = %#v", properties)
	}
	pids, ok := properties["PIDs"].([]uint32)
	if !ok || len(pids) != 1 || pids[0] != 1234 {
		t.Fatalf("PIDs property = %#v", properties["PIDs"])
	}
}

func TestStartDelegatedScopeRejectsFailedJob(t *testing.T) {
	fake := &fakeTransientScopeStarter{result: "failed"}
	if err := startDelegatedScope(context.Background(), fake, "brun-20260716-120000-abcdef.scope", 1234); err == nil {
		t.Fatal("failed systemd job was accepted")
	}
}
