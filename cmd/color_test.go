package cmd

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPadRightUsesTerminalCellWidth(t *testing.T) {
	got := PadRight("分析", 6)
	if width := visibleWidth(got); width != 6 {
		t.Fatalf("visibleWidth(PadRight()) = %d, want 6; output %q", width, got)
	}
}

func TestPadRightTruncatesUTF8AtCellBoundary(t *testing.T) {
	got := PadRight("基因组分析", 7)
	if !utf8.ValidString(got) {
		t.Fatalf("PadRight() returned invalid UTF-8: %q", got)
	}
	if width := visibleWidth(got); width != 7 {
		t.Fatalf("visibleWidth(PadRight()) = %d, want 7; output %q", width, got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("PadRight() = %q, want ellipsis suffix", got)
	}
}

func TestTableHeaderFormatsBeforeAddingANSI(t *testing.T) {
	oldUseColor := useColor
	useColor = true
	defer func() { useColor = oldUseColor }()

	got := TableHeader("%-10s %-8s\n", "FIRST", "SECOND")
	plain := stripANSI(got)
	if index := strings.Index(plain, "SECOND"); index != 11 {
		t.Fatalf("SECOND starts at column %d, want 11; output %q", index, plain)
	}
}

func TestSemanticColors(t *testing.T) {
	oldUseColor := useColor
	useColor = true
	defer func() { useColor = oldUseColor }()

	if got, want := StatusColor("success(+warnings)"), Green("success")+Yellow("(+warnings)"); got != want {
		t.Fatalf("StatusColor() = %q, want %q", got, want)
	}
	if got, want := StatusColor("failed(+warnings)"), Red("failed")+Yellow("(+warnings)"); got != want {
		t.Fatalf("StatusColor() = %q, want %q", got, want)
	}
	if got := DiagnosticColor("E2"); got != Red("E2") {
		t.Fatalf("DiagnosticColor(E2) = %q, want red", got)
	}
	if got := DiagnosticColor("W1"); got != Yellow("W1") {
		t.Fatalf("DiagnosticColor(W1) = %q, want yellow", got)
	}
	if got := stripANSI(StatusColor("cancelled")); got != "cancelled" {
		t.Fatalf("StatusColor(cancelled) text = %q", got)
	}
	if got := StatusColor("timed_out"); got != Red("timed_out") {
		t.Fatalf("StatusColor(timed_out) = %q, want red", got)
	}
}

func TestSemanticColorsDisabled(t *testing.T) {
	oldUseColor := useColor
	useColor = false
	defer func() { useColor = oldUseColor }()

	if got := StatusColor("success(+warnings)"); got != "success(+warnings)" {
		t.Fatalf("StatusColor() with color disabled = %q", got)
	}
	if got := DetailLabelColor("cwd"); got != "cwd" {
		t.Fatalf("DetailLabelColor() with color disabled = %q", got)
	}
}
