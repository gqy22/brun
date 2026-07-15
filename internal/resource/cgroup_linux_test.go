//go:build linux

package resource

import (
	"testing"

	cgroupstats "github.com/containerd/cgroups/v3/cgroup2/stats"
)

func TestStatsFromMetrics(t *testing.T) {
	metrics := &cgroupstats.Metrics{
		CPU:    &cgroupstats.CPUStat{UsageUsec: 9_500, UserUsec: 7_000, SystemUsec: 2_500},
		Memory: &cgroupstats.MemoryStat{MaxUsage: 4096},
		Io: &cgroupstats.IOStat{Usage: []*cgroupstats.IOEntry{
			{Rbytes: 100, Wbytes: 20},
			{Rbytes: 50, Wbytes: 30},
		}},
		MemoryEvents: &cgroupstats.MemoryEvents{OomKill: 2},
		Pids:         &cgroupstats.PidsStat{Current: 3},
	}
	got := statsFromMetrics(metrics)
	if got.CPUTimeMs != 9 || got.CPUUserMs != 7 || got.CPUSystemMs != 2 {
		t.Fatalf("cpu stats = %+v", got)
	}
	if got.MemoryPeakByte != 4096 || got.IOReadBytes != 150 || got.IOWriteBytes != 50 || got.OOMKillCount != 2 || got.PIDsPeak != 3 {
		t.Fatalf("resource stats = %+v", got)
	}
}

func TestValidRunID(t *testing.T) {
	if !validRunID("20260715-120000-abcdef") {
		t.Fatal("valid run id rejected")
	}
	for _, value := range []string{"short", "../escape", "UPPERCASE-ID"} {
		if validRunID(value) {
			t.Fatalf("invalid run id accepted: %q", value)
		}
	}
}
