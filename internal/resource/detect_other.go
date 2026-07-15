//go:build !linux

package resource

func ProcSupported() bool { return false }

func Detect() Environment { return Environment{Reason: "not_linux"} }
