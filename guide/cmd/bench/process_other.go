//go:build !linux

package main

import "os/exec"

func configureManagedCommand(_ *exec.Cmd) {}
