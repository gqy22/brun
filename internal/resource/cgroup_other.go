//go:build !linux

package resource

import (
	"fmt"
	"time"
)

type CgroupScope struct{}

func NewCgroupScope(Environment, string) (*CgroupScope, error) {
	return nil, fmt.Errorf("cgroup v2 is only supported on Linux")
}

func LoadCgroupScope(string) (*CgroupScope, error) {
	return nil, fmt.Errorf("cgroup v2 is only supported on Linux")
}

type CgroupTermination struct {
	Empty     bool
	Signal    string
	Escalated bool
}

func TerminateCgroup(string, time.Duration, bool) (CgroupTermination, error) {
	return CgroupTermination{}, fmt.Errorf("cgroup v2 is only supported on Linux")
}
