//go:build !linux

package resource

import "fmt"

func AcquireDelegation(string) (Environment, error) {
	env := Detect()
	return env, fmt.Errorf("automatic cgroup delegation unavailable: %s", env.Reason)
}
