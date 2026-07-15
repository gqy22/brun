//go:build linux

package resource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func ProcSupported() bool { return true }

func Detect() Environment {
	return detectLinux("/proc/self/cgroup", "/sys/fs/cgroup")
}

func detectLinux(procFile, mountpoint string) Environment {
	env := Environment{Mountpoint: mountpoint}
	var stat unix.Statfs_t
	if err := unix.Statfs(mountpoint, &stat); err != nil || stat.Type != unix.CGROUP2_SUPER_MAGIC {
		env.Reason = "cgroup_v2_unavailable"
		return env
	}
	env.Unified = true
	data, err := os.ReadFile(procFile)
	if err != nil {
		env.Reason = "cgroup_path_unavailable"
		return env
	}
	path, err := parseUnifiedPath(data)
	if err != nil {
		env.Reason = "cgroup_path_unavailable"
		return env
	}
	env.CurrentPath = path
	full, err := safeCgroupPath(mountpoint, path)
	if err != nil {
		env.Reason = "cgroup_path_invalid"
		return env
	}
	env.FullPath = full
	if controllers, err := os.ReadFile(filepath.Join(full, "cgroup.controllers")); err == nil {
		env.Controllers = strings.Fields(string(controllers))
	}
	if delegatedByXattr(full) || delegatedByProbe(full) {
		env.Delegated = true
		return env
	}
	env.Reason = "not_delegated"
	return env
}

func parseUnifiedPath(data []byte) (string, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "0::") {
			path := strings.TrimPrefix(line, "0::")
			if path == "" || strings.Contains(path, " (deleted)") {
				break
			}
			return path, nil
		}
	}
	return "", fmt.Errorf("unified cgroup entry not found")
}

func safeCgroupPath(mountpoint, cgroupPath string) (string, error) {
	if !filepath.IsAbs(cgroupPath) {
		return "", fmt.Errorf("cgroup path is not absolute")
	}
	root := filepath.Clean(mountpoint)
	full := filepath.Join(root, strings.TrimPrefix(filepath.Clean(cgroupPath), string(filepath.Separator)))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cgroup path escapes mountpoint")
	}
	return full, nil
}

func delegatedByXattr(path string) bool {
	buf := make([]byte, 8)
	n, err := unix.Getxattr(path, "user.delegate", buf)
	return err == nil && string(buf[:n]) == "1"
}

func delegatedByProbe(path string) bool {
	probe, err := os.MkdirTemp(path, ".brun-delegate-probe-")
	if err != nil {
		return false
	}
	return os.Remove(probe) == nil
}
