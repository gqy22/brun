//go:build !linux

package resource

import "fmt"

type CgroupScope struct{}

func NewCgroupScope(Environment, string) (*CgroupScope, error) {
	return nil, fmt.Errorf("cgroup v2 is only supported on Linux")
}
