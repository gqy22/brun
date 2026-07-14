package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
)

var useColor = isTerminal(os.Stdout.Fd())

const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	gray    = "\033[90m"
)

func isTerminal(fd uintptr) bool {
	// 简单检测：如果 NO_COLOR 环境变量存在，禁用颜色
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// 检查 stdout 是否为终端
	if f, ok := os.Stdout.Stat(); ok == nil && (f.Mode()&os.ModeCharDevice) != 0 {
		return true
	}
	return false
}

func colorize(color, s string) string {
	if !useColor || s == "" {
		return s
	}
	body := strings.TrimRight(s, "\r\n")
	return color + body + reset + s[len(body):]
}

func Bold(s string) string    { return colorize(bold, s) }
func Red(s string) string     { return colorize(red, s) }
func Green(s string) string   { return colorize(green, s) }
func Yellow(s string) string  { return colorize(yellow, s) }
func Blue(s string) string    { return colorize(blue, s) }
func Magenta(s string) string { return colorize(magenta, s) }
func Cyan(s string) string    { return colorize(cyan, s) }
func Gray(s string) string    { return colorize(gray, s) }
func Dim(s string) string     { return colorize(dim, s) }

// StatusColor 根据状态返回带颜色的字符串
func StatusColor(status string) string {
	base, warningSuffix := status, ""
	if i := strings.Index(status, "(+"); i >= 0 {
		base, warningSuffix = status[:i], status[i:]
	}

	var styledBase string
	switch base {
	case "success":
		styledBase = Green(base)
	case "failed":
		styledBase = Red(base)
	case "running":
		styledBase = Yellow(base)
	case "cancelled":
		styledBase = Magenta(base)
	default:
		styledBase = Cyan(base)
	}
	if warningSuffix != "" {
		return styledBase + Yellow(warningSuffix)
	}
	return styledBase
}

func RunIDColor(id string) string {
	return Gray(id)
}

func NameColor(name string) string {
	return Bold(name)
}

func ProjectColor(project string) string {
	return Cyan(project)
}

func DiagnosticColor(diagnostic string) string {
	if diagnostic == "" || diagnostic == "-" {
		return Dim(diagnostic)
	}
	if strings.HasPrefix(diagnostic, "E") {
		return Red(diagnostic)
	}
	if strings.HasPrefix(diagnostic, "W") {
		return Yellow(diagnostic)
	}
	return Dim(diagnostic)
}

func DurationColor(status, duration string) string {
	if status == "running" {
		return Yellow(duration)
	}
	return Dim(duration)
}

func DetailLabelColor(label string) string {
	if label == "cwd" {
		return Blue(label)
	}
	return Cyan(label)
}

// KindColor 根据文件类型返回带颜色的字符串
func KindColor(kind string) string {
	switch kind {
	case "input":
		return Cyan(kind)
	case "output":
		return Green(kind)
	case "script":
		return Yellow(kind)
	case "config":
		return Dim(kind)
	case "report":
		return Bold(kind)
	default:
		return kind
	}
}

// PadRight 右对齐填充（用于表格列）
func PadRight(s string, width int) string {
	visibleLen := visibleWidth(s)
	if visibleLen > width {
		plain := stripANSI(s)
		truncated := runewidth.Truncate(plain, width, "...")
		if plain != s {
			// brun's styled cells wrap the whole value in one leading SGR sequence.
			// Preserve that style while ensuring a reset survives truncation.
			if end := strings.IndexByte(s, 'm'); strings.HasPrefix(s, "\033[") && end >= 0 {
				return s[:end+1] + truncated + reset
			}
		}
		return truncated
	}
	padding := width - visibleLen
	return s + strings.Repeat(" ", padding)
}

// visibleWidth returns terminal cell width after removing ANSI escape sequences.
func visibleWidth(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) {
				c := s[i]
				i++
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// TableHeader 返回加粗的表头行
func TableHeader(format string, args ...interface{}) string {
	// Format plain text first. Adding ANSI to individual arguments before fmt
	// makes fmt count invisible escape bytes as part of each field width.
	return Bold(fmt.Sprintf(format, args...))
}
