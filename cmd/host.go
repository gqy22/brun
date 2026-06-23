package cmd

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

// cpuSampler 后台定期采样 CPU 使用率。
//
// gopsutil 的 cpu.Percent(interval, false) 在 interval>0 时会阻塞 interval 秒
// 做两次采样，因此不能在 HTTP handler 里直接调用，否则每个请求都会卡住。
// 这里用一个常驻 goroutine 周期采样并缓存，handler 直接读缓存即可。
type cpuSampler struct {
	mu      sync.Mutex
	percent float64
	known   bool // 首次样本就绪前为 false
	stop    chan struct{}
}

// startCPUSampler 启动后台 goroutine，每 interval 调用一次 cpu.Percent 并缓存结果。
func startCPUSampler(interval time.Duration) *cpuSampler {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	c := &cpuSampler{stop: make(chan struct{})}
	go c.loop(interval)
	return c
}

func (c *cpuSampler) loop(interval time.Duration) {
	sample := func() {
		// cpu.Percent(time.Second, false) 阻塞约 1s，返回总体使用率
		p, err := cpu.Percent(time.Second, false)
		if err == nil && len(p) > 0 {
			c.mu.Lock()
			c.percent = p[0]
			c.known = true
			c.mu.Unlock()
		}
	}
	sample() // 预热：立即采一次，避免首次请求长时间拿到 unknown
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			sample()
		}
	}
}

// Snapshot 返回最近一次采样的 CPU 使用率（0-100）以及是否已有有效样本。
func (c *cpuSampler) Snapshot() (percent float64, known bool) {
	if c == nil {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.percent, c.known
}

// Stop 停止后台采样。进程退出时无需调用，goroutine 会随之结束。
func (c *cpuSampler) Stop() {
	if c == nil {
		return
	}
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}

// --- HostStats 数据结构 ---

type HostCPU struct {
	UsedPercent float64 `json:"used_percent"`
	Cores       int     `json:"cores"`
	Known       bool    `json:"known"`
}

type HostMemory struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"used_percent"`
}

type HostLoad struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type HostDisk struct {
	Device      string  `json:"device"`
	Mountpoint  string  `json:"mountpoint"`
	Fstype      string  `json:"fstype"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

type HostStats struct {
	Hostname string     `json:"hostname"`
	Platform string     `json:"platform"`
	Uptime   uint64     `json:"uptime_sec"`
	CPU      HostCPU    `json:"cpu"`
	Memory   HostMemory `json:"memory"`
	Swap     HostMemory `json:"swap"`
	Load     HostLoad   `json:"load"`
	Disks    []HostDisk `json:"disks"`
}

// gatherHostStats 收集主机各项资源指标。cpuPercent/cpuKnown 由后台 cpuSampler 提供，
// 其余指标即时采集；每项独立容错，单项失败留零值不影响整体。
func gatherHostStats(cpuPercent float64, cpuKnown bool) HostStats {
	stats := HostStats{
		CPU: HostCPU{
			UsedPercent: cpuPercent,
			Known:       cpuKnown,
		},
	}

	if cores, err := cpu.Counts(true); err == nil {
		stats.CPU.Cores = cores
	}

	if h, err := host.Info(); err == nil {
		stats.Hostname = h.Hostname
		stats.Platform = strings.TrimSpace(h.Platform + " " + h.PlatformVersion)
		stats.Uptime = h.Uptime
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		stats.Memory = HostMemory{
			Total:       vm.Total,
			Used:        vm.Used,
			Available:   vm.Available,
			UsedPercent: vm.UsedPercent,
		}
	}

	if sw, err := mem.SwapMemory(); err == nil {
		stats.Swap = HostMemory{
			Total:       sw.Total,
			Used:        sw.Used,
			Available:   sw.Total - sw.Used,
			UsedPercent: sw.UsedPercent,
		}
	}

	if l, err := load.Avg(); err == nil {
		stats.Load = HostLoad{Load1: l.Load1, Load5: l.Load5, Load15: l.Load15}
	}

	// disk.Partitions(false) 只列物理设备，自动过滤 tmpfs/proc/sysfs 等虚拟文件系统。
	// 网络文件系统（NFS/CIFS/...）和 FUSE 挂载点需在此额外排除：对它们调用 disk.Usage
	// 会触发 Statfs，在挂载不可达（如 stale NFS）时可能无限期阻塞，拖垮整个请求。
	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			if isBlockedFSType(p.Fstype) {
				continue
			}
			u := diskUsageSafe(p.Mountpoint, 2*time.Second)
			if u == nil {
				continue
			}
			stats.Disks = append(stats.Disks, HostDisk{
				Device:      p.Device,
				Mountpoint:  p.Mountpoint,
				Fstype:      p.Fstype,
				Total:       u.Total,
				Used:        u.Used,
				Free:        u.Free,
				UsedPercent: u.UsedPercent,
			})
		}
	}

	return stats
}

// blockedFSTypes 列出可能导致 Statfs 阻塞的文件系统类型。对这类挂载点调用
// disk.Usage 可能无限期挂起（典型如 stale NFS），必须直接跳过。
var blockedFSTypes = map[string]bool{
	"nfs": true, "nfs4": true,
	"cifs": true, "smbfs": true, "smb2": true, "smb3": true,
	"glusterfs": true, "ceph": true, "9p": true, "lustre": true,
	"autofs": true,
}

func isBlockedFSType(fstype string) bool {
	if blockedFSTypes[fstype] {
		return true
	}
	// 所有 fuse 子类型（fuse.sshfs / fuse.gvfsd-fuse 等）一律跳过
	return strings.HasPrefix(fstype, "fuse")
}

// diskUsageSafe 带超时地查询挂载点用量。Statfs 对异常挂载点可能阻塞，
// 超时则放弃该挂载点（goroutine 在 syscall 返回后自然退出），避免拖垮整个请求。
func diskUsageSafe(mountpoint string, timeout time.Duration) *disk.UsageStat {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	type res struct {
		u   *disk.UsageStat
		err error
	}
	ch := make(chan res, 1)
	go func() {
		u, err := disk.UsageWithContext(ctx, mountpoint)
		ch <- res{u, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil
		}
		return r.u
	case <-time.After(timeout):
		return nil
	}
}
