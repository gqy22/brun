package resource

import "testing"

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
