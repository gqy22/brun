package cmd

import (
	"runtime"
	"testing"
	"time"
)

func TestGatherHostStats(t *testing.T) {
	stats := gatherHostStats(42.5, true)

	// 注入的 CPU 值应原样返回
	if stats.CPU.UsedPercent != 42.5 {
		t.Errorf("cpu.used_percent = %v, want 42.5", stats.CPU.UsedPercent)
	}
	if !stats.CPU.Known {
		t.Errorf("cpu.known = false, want true")
	}

	// Linux 上内存总量、CPU 核数应能采到
	if runtime.GOOS == "linux" && stats.Memory.Total == 0 {
		t.Errorf("memory.total = 0 on linux, want > 0")
	}
	if runtime.GOOS == "linux" && stats.CPU.Cores <= 0 {
		t.Errorf("cpu.cores = %v on linux, want > 0", stats.CPU.Cores)
	}

	t.Logf("hostname=%s platform=%s mem.total=%d disks=%d load=%.2f/%.2f/%.2f",
		stats.Hostname, stats.Platform, stats.Memory.Total,
		len(stats.Disks), stats.Load.Load1, stats.Load.Load5, stats.Load.Load15)
}

// TestCPUSampler 验证后台采样 goroutine 能在预热后产出有效的 CPU 使用率样本。
func TestCPUSampler(t *testing.T) {
	s := startCPUSampler(2 * time.Second)
	defer s.Stop()

	// 启动后立即调用 Snapshot 不应 panic（预热样本可能尚未就绪）
	_, _ = s.Snapshot()

	// cpu.Percent 阻塞约 1s，给它一点时间完成首次采样
	time.Sleep(1500 * time.Millisecond)

	p, known := s.Snapshot()
	if !known {
		t.Fatalf("cpu sampler 未在 1.5s 内产出样本")
	}
	if p < 0 || p > 100 {
		t.Errorf("cpu percent = %v, want [0,100]", p)
	}
}
