package resource

import (
	"errors"
	"testing"
)

func TestDecide(t *testing.T) {
	delegated := Environment{Unified: true, Delegated: true, CurrentPath: "/scope"}
	got, err := decide(ModeAuto, delegated, true)
	if err != nil || got.Backend != BackendCgroupV2 || got.Fallback != "" {
		t.Fatalf("delegated auto decision = %+v, %v", got, err)
	}

	unavailable := Environment{Reason: "not_delegated"}
	got, err = decide(ModeAuto, unavailable, true)
	if err != nil || got.Backend != BackendProc || got.Fallback != "not_delegated" {
		t.Fatalf("fallback decision = %+v, %v", got, err)
	}
	if _, err := decide(ModeCgroup, unavailable, true); err == nil {
		t.Fatal("forced cgroup succeeded without delegation")
	}
}

func TestResolveAcquiresDelegationBeforeSelectingBackend(t *testing.T) {
	initial := Environment{Unified: true, Reason: "not_delegated"}
	delegated := Environment{Unified: true, Delegated: true, CurrentPath: "/scope"}
	called := false
	got, err := resolve(ModeAuto, "20260716-120000-abcdef", initial, true, func(runID string) (Environment, error) {
		called = true
		return delegated, nil
	})
	if err != nil || !called || got.Backend != BackendCgroupV2 || got.Env.CurrentPath != "/scope" {
		t.Fatalf("resolve auto = %+v, %v; called=%t", got, err, called)
	}
}

func TestResolveAutoFallsBackWhenSystemdScopeFails(t *testing.T) {
	initial := Environment{Unified: true, Reason: "not_delegated"}
	got, err := resolve(ModeAuto, "20260716-120000-abcdef", initial, true, func(string) (Environment, error) {
		return Environment{}, errors.New("no user bus")
	})
	if err != nil || got.Backend != BackendProc || got.Fallback != "systemd_scope_unavailable" || got.Detail != "no user bus" {
		t.Fatalf("resolve auto fallback = %+v, %v", got, err)
	}
}

func TestResolveCgroupRejectsFailedSystemdScope(t *testing.T) {
	initial := Environment{Unified: true, Reason: "not_delegated"}
	_, err := resolve(ModeCgroup, "20260716-120000-abcdef", initial, true, func(string) (Environment, error) {
		return Environment{}, errors.New("denied")
	})
	if err == nil {
		t.Fatal("forced cgroup accepted failed automatic delegation")
	}
}
