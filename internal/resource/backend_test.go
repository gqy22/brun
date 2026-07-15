package resource

import "testing"

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
