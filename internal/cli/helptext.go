package cli

import (
	"embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed help/*.md
var helpFS embed.FS

// HelpText holds parsed help data from a single markdown file.
type HelpText struct {
	Use     string
	Short   string
	Long    string
	Example string
	Output  string
	Aliases []string
}

// Parse reads a help markdown file from the embedded FS by name (without .md extension).
func Parse(name string) (*HelpText, error) {
	data, err := helpFS.ReadFile("help/" + name + ".md")
	if err != nil {
		return nil, fmt.Errorf("help file %q: %w", name, err)
	}
	return ParseBytes(data)
}

// ParseBytes parses raw markdown bytes with YAML-like frontmatter.
//
// Expected format:
//
//	---
//	use: "command -- <args>"
//	short: "单行描述"
//	long: |
//	  多行详细描述...
//	example: |
//	  # 示例
//	  brun command ...
//	output: |
//	  ## 输出格式
//	  ...
//	---
//	可选的 body 文本（作为 long 的 fallback）
func ParseBytes(data []byte) (*HelpText, error) {
	lines := strings.Split(string(data), "\n")
	var ht HelpText

	// Find frontmatter boundaries
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing frontmatter opening ---")
	}

	fmEnd := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			fmEnd = i
			break
		}
	}
	if fmEnd == -1 {
		return nil, fmt.Errorf("missing frontmatter closing ---")
	}

	// Parse frontmatter key-value pairs
	i := 1
	for i < fmEnd {
		line := lines[i]
		// Skip empty lines within frontmatter
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		// Parse key: value or key: | (block scalar)
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			i++
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		rest := strings.TrimSpace(line[colonIdx+1:])

		switch key {
		case "use":
			ht.Use = unquote(rest)
		case "short":
			ht.Short = unquote(rest)
		case "aliases":
			// Parse list items on subsequent lines
			if rest != "" {
				ht.Aliases = append(ht.Aliases, unquote(rest))
			}
			i++
			for i < fmEnd && strings.HasPrefix(strings.TrimSpace(lines[i]), "-") {
				item := strings.TrimSpace(lines[i][1:])
				ht.Aliases = append(ht.Aliases, strings.TrimSpace(item))
				i++
			}
			continue // already advanced
		case "long", "example", "output":
			// Block scalar: collect indented lines until non-indented or end of FM
			if rest == "|" {
				i++
				var block []string
				for i < fmEnd {
					l := lines[i]
					if strings.TrimSpace(l) == "" || (len(l) > 0 && (l[0] == ' ' || l[0] == '\t')) {
						block = append(block, l)
						i++
					} else {
						break
					}
				}
				val := strings.TrimRight(strings.Join(block, "\n"), "\n")
				val = strings.TrimLeft(val, " \t") // remove uniform indent
				switch key {
				case "long":
					ht.Long = val
				case "example":
					ht.Example = val
				case "output":
					ht.Output = val
				}
				continue // already advanced
			}
			// Inline value (no block scalar marker)
			switch key {
			case "long":
				ht.Long = unquote(rest)
			case "example":
				ht.Example = unquote(rest)
			case "output":
				ht.Output = unquote(rest)
			}
		default:
			// Ignore unknown keys
		}
		i++
	}

	// Body after frontmatter becomes Long fallback
	if ht.Long == "" && fmEnd+1 < len(lines) {
		bodyLines := lines[fmEnd+1:]
		// Trim leading/trailing blank lines
		for len(bodyLines) > 0 && strings.TrimSpace(bodyLines[0]) == "" {
			bodyLines = bodyLines[1:]
		}
		for len(bodyLines) > 0 && strings.TrimSpace(bodyLines[len(bodyLines)-1]) == "" {
			bodyLines = bodyLines[:len(bodyLines)-1]
		}
		ht.Long = strings.Join(bodyLines, "\n")
	}

	// Validate required fields
	if ht.Use == "" {
		return nil, fmt.Errorf("required field 'use' is empty")
	}
	if ht.Short == "" {
		return nil, fmt.Errorf("required field 'short' is empty")
	}

	return &ht, nil
}

// unquote removes surrounding double quotes if present.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// Inject applies parsed help fields onto a cobra.Command.
func (ht *HelpText) Inject(cmd *cobra.Command) {
	cmd.Use = ht.Use
	cmd.Short = ht.Short
	cmd.Long = ht.Long
	cmd.Example = ht.Example
	if len(ht.Aliases) > 0 {
		cmd.Aliases = ht.Aliases
	}
	if ht.Output != "" {
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string)
		}
		cmd.Annotations["output"] = ht.Output
	}
}

// MustParse calls Parse and panics on error. Use at init-time in command factories.
func MustParse(name string) *HelpText {
	ht, err := Parse(name)
	if err != nil {
		panic("brun helptext: " + err.Error())
	}
	return ht
}

// ---------------------------------------------------------------------------
// Template constants — new render order:
//   Description → Usage → [Subcommands] → Options → [Global Options] → Examples → [Output]
// ---------------------------------------------------------------------------

const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}
{{if .Example}}

示例:
{{.Example}}
{{end}}
{{with index .Annotations "output"}}

输出:
{{.}}
{{end}}`

const usageTemplate = `用法: {{.UseLine}}

{{if .HasAvailableSubCommands}}
可用命令:
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}
{{end}}
{{if .HasAvailableLocalFlags}}
选项:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
{{if .HasAvailableInheritedFlags}}
全局选项:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}
{{if .HasHelpSubCommands}}
更多帮助命令:
{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}
{{end}}
{{if .HasAvailableSubCommands}}
使用 "{{.CommandPath}} [命令] --help" 获取更多信息
{{end}}
`
