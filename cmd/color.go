package cmd

import (
	"fmt"
	"os"
	"strings"
)

var useColor = isTerminal(os.Stdout.Fd())

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	gray   = "\033[90m"
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
	return color + s + reset
}

func Bold(s string) string   { return colorize(bold, s) }
func Red(s string) string    { return colorize(red, s) }
func Green(s string) string  { return colorize(green, s) }
func Yellow(s string) string { return colorize(yellow, s) }
func Cyan(s string) string   { return colorize(cyan, s) }
func Gray(s string) string   { return colorize(gray, s) }
func Dim(s string) string    { return colorize(dim, s) }

// StatusColor 根据状态返回带颜色的字符串
func StatusColor(status string) string {
	switch status {
	case "success":
		return Green(status)
	case "failed":
		return Red(status)
	case "running":
		return Yellow(status)
	default:
		return Cyan(status)
	}
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
		return dim + kind + reset
	case "report":
		return bold + kind + reset
	default:
		return kind
	}
}

// PadRight 右对齐填充（用于表格列）
func PadRight(s string, width int) string {
	if len(s) >= width {
		if width > 3 {
			return s[:width-3] + "..."
		}
		return s[:width]
	}
	// 计算可见字符宽度（去除 ANSI 转义序列）
	visibleLen := visibleWidth(s)
	padding := width - visibleLen
	return s + strings.Repeat(" ", padding)
}

// visibleWidth 计算去除 ANSI 转义后的可见宽度
func visibleWidth(s string) int {
	width := 0
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		width++
	}
	return width
}

// TableHeader 返回加粗的表头行
func TableHeader(format string, args ...interface{}) string {
	boldArgs := make([]interface{}, len(args))
	for i, a := range args {
		if s, ok := a.(string); ok {
			boldArgs[i] = Bold(s)
		} else {
			boldArgs[i] = a
		}
	}
	return fmt.Sprintf(format, boldArgs...)
}
