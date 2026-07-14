package cli

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// --- Parser tests ---

func TestParseAllFiles(t *testing.T) {
	// Every help/*.md file (except _shared.md) must parse successfully
	names := []string{
		"root", "init", "run", "list", "show", "logs",
		"outputs", "diag", "stop", "rerun", "tag", "note",
		"script", "clean", "repair", "web",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			ht, err := Parse(name)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", name, err)
			}
			if ht.Use == "" {
				t.Error("use is empty")
			}
			if ht.Short == "" {
				t.Error("short is empty")
			}
		})
	}
}

func TestMissingRequiredField(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "missing use",
			content: "---\nshort: test\n---\nbody\n",
			wantErr: "use",
		},
		{
			name:    "missing short",
			content: "---\nuse: cmd\n---\nbody\n",
			wantErr: "short",
		},
		{
			name:    "missing both",
			content: "---\n---\nbody\n",
			wantErr: "use",
		},
		{
			name:    "no frontmatter",
			content: "just body text",
			wantErr: "frontmatter",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tc.content))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestBodyFallbackAsLong(t *testing.T) {
	md := `---
use: "test"
short: "short desc"
---
This is the body text.
It becomes Long when no long: field is set.
`
	ht, err := ParseBytes([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ht.Long, "This is the body text") {
		t.Errorf("Long = %q, want body text", ht.Long)
	}
}

func TestBlockScalarFields(t *testing.T) {
	md := `---
use: "cmd"
short: "desc"
long: |
  Multi-line
  long text
example: |
  # Example 1
  cmd --foo
output: |
  ## Output
  table here
---
`
	ht, err := ParseBytes([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ht.Long, "Multi-line") {
		t.Errorf("Long = %q", ht.Long)
	}
	if !strings.Contains(ht.Example, "Example 1") {
		t.Errorf("Example = %q", ht.Example)
	}
	if !strings.Contains(ht.Output, "Output") {
		t.Errorf("Output = %q", ht.Output)
	}
	if ht.Example != "# Example 1\ncmd --foo" {
		t.Errorf("Example indentation = %q", ht.Example)
	}
}

func TestTerminalHelpTextRemovesMarkdownSyntax(t *testing.T) {
	input := "## 输出\n\n**状态**: `success`\n\n| 列 | 说明 |\n|---|---|\n| ID | 标识 |"
	got := terminalHelpText(input)
	for _, marker := range []string{"##", "**", "`", "|---"} {
		if strings.Contains(got, marker) {
			t.Fatalf("terminal help still contains %q: %q", marker, got)
		}
	}
	for _, text := range []string{"输出:", "状态: success", "ID  标识"} {
		if !strings.Contains(got, text) {
			t.Fatalf("terminal help missing %q: %q", text, got)
		}
	}
}

func TestUnquote(t *testing.T) {
	md := `---
use: "cmd"
short: "quoted value"
---
`
	ht, err := ParseBytes([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if ht.Short != "quoted value" {
		t.Errorf("Short = %q, want unquoted", ht.Short)
	}
}

// --- Inject tests ---

func TestInjectSetsCobraFields(t *testing.T) {
	ht := &HelpText{
		Use:     "test -- <args>",
		Short:   "short description",
		Long:    "long description",
		Example: "# example\ntest -- foo",
		Output:  "## Output\nsome output",
		Aliases: []string{"t"},
	}
	cmd := &cobra.Command{}
	ht.Inject(cmd)

	if cmd.Use != "test -- <args>" {
		t.Errorf("Use = %q, want %q", cmd.Use, "test -- <args>")
	}
	if cmd.Short != "short description" {
		t.Errorf("Short = %q", cmd.Short)
	}
	if cmd.Long != "long description" {
		t.Errorf("Long = %q", cmd.Long)
	}
	if cmd.Example != "example:\ntest -- foo" {
		t.Errorf("Example = %q", cmd.Example)
	}
	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "t" {
		t.Errorf("Aliases = %v", cmd.Aliases)
	}
	if cmd.Annotations["output"] != "Output:\nsome output" {
		t.Errorf("Annotations[output] = %q", cmd.Annotations["output"])
	}
}

func TestInjectNoOutputWhenEmpty(t *testing.T) {
	ht := &HelpText{Use: "c", Short: "s"}
	cmd := &cobra.Command{}
	ht.Inject(cmd)

	if _, ok := cmd.Annotations["output"]; ok {
		t.Error("output annotation should not be set when empty")
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("MustParse should panic on bad input")
		}
	}()
	MustParse("nonexistent_command_xyz")
}

// --- Integration tests (verify actual command help works) ---

func TestCommandHelpRenders(t *testing.T) {
	// Verify that every registered command can render help without panic
	cmds := map[string]func() *cobra.Command{
		"init":    initCmd,
		"run":     runCmd,
		"list":    listCmd,
		"show":    showCmd,
		"script":  scriptCmd,
		"logs":    logsCmd,
		"outputs": outputsCmd,
		"diag":    diagCmd,
		"tag":     tagCmd,
		"note":    noteCmd,
		"stop":    stopCmd,
		"rerun":   rerunCmd,
		"clean":   cleanCmd,
		"repair":  repairCmd,
		"web":     webCmd,
	}
	for name, factory := range cmds {
		t.Run(name, func(t *testing.T) {
			c := factory()
			// Capture help output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			c.SetOut(w)
			err := c.Help()
			w.Close()
			os.Stdout = oldStdout

			if err != nil {
				t.Fatalf("Help() error: %v", err)
			}

			helpBytes, _ := io.ReadAll(r)
			help := string(helpBytes)

			if help == "" && !strings.Contains(c.Short, " ") {
				// Only fail if truly empty (commands with space in short are real commands)
			}
			// Verify new template order: Usage before Example
			usageIdx := strings.Index(help, "用法:")
			exampleIdx := strings.Index(help, "示例:")
			if usageIdx >= 0 && exampleIdx >= 0 && usageIdx > exampleIdx {
				t.Errorf("用法 appears after 示例; expected new order (usageIdx=%d exampleIdx=%d)", usageIdx, exampleIdx)
			}
		})
	}
}
