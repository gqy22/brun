package resource

import (
	"testing"
	"time"
)

func TestParseMode(t *testing.T) {
	for input, want := range map[string]Mode{"": ModeAuto, "AUTO": ModeAuto, "proc": ModeProc, "cgroup": ModeCgroup} {
		got, err := ParseMode(input)
		if err != nil || got != want {
			t.Fatalf("ParseMode(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := ParseMode("magic"); err == nil {
		t.Fatal("ParseMode accepted an unknown backend")
	}
}

type fakeScope struct {
	populated []bool
	index     int
}

func (f *fakeScope) Backend() string       { return "fake" }
func (f *fakeScope) Path() string          { return "/fake" }
func (f *fakeScope) Attach(int) error      { return nil }
func (f *fakeScope) Verify(int) error      { return nil }
func (f *fakeScope) Stats() (Stats, error) { return Stats{}, nil }
func (f *fakeScope) Kill() error           { return nil }
func (f *fakeScope) Close() error          { return nil }
func (f *fakeScope) Populated() (bool, error) {
	value := f.populated[f.index]
	if f.index < len(f.populated)-1 {
		f.index++
	}
	return value, nil
}

func TestWaitEmpty(t *testing.T) {
	scope := &fakeScope{populated: []bool{true, true, false}}
	empty, err := WaitEmpty(scope, time.Second)
	if err != nil || !empty {
		t.Fatalf("WaitEmpty() = %t, %v", empty, err)
	}
	scope = &fakeScope{populated: []bool{true}}
	empty, err = WaitEmpty(scope, 0)
	if err != nil || empty {
		t.Fatalf("immediate WaitEmpty() = %t, %v", empty, err)
	}
}
