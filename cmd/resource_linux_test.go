//go:build linux

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseProcStatHandlesCommandNamesWithSpacesAndParens(t *testing.T) {
	stat := makeProcStat(123, "python (worker) 1", 777, 41, 9)

	got, err := parseProcStat([]byte(stat))
	if err != nil {
		t.Fatalf("parseProcStat returned error: %v", err)
	}
	if got.pid != 123 {
		t.Fatalf("pid = %d, want 123", got.pid)
	}
	if got.pgrp != 777 {
		t.Fatalf("pgrp = %d, want 777", got.pgrp)
	}
	if got.utimeTicks != 41 {
		t.Fatalf("utimeTicks = %d, want 41", got.utimeTicks)
	}
	if got.stimeTicks != 9 {
		t.Fatalf("stimeTicks = %d, want 9", got.stimeTicks)
	}
}

func TestSampleProcessGroupFromProcAggregatesMatchingProcessGroup(t *testing.T) {
	root := t.TempDir()
	writeFakeProc(t, root, 101, makeProcStat(101, "bash script", 777, 30, 10), "VmRSS:\t1200 kB\n")
	writeFakeProc(t, root, 102, makeProcStat(102, "python worker", 777, 50, 20), "VmRSS:\t3000 kB\n")
	writeFakeProc(t, root, 201, makeProcStat(201, "other", 999, 100, 100), "VmRSS:\t9999 kB\n")
	if err := os.Mkdir(filepath.Join(root, "self"), 0755); err != nil {
		t.Fatal(err)
	}

	got := sampleProcessGroupFromProc(root, 777, 100)
	if got.PeakRSSKB != 4200 {
		t.Fatalf("PeakRSSKB = %d, want 4200", got.PeakRSSKB)
	}
	if got.CPUTimeMs != 1100 {
		t.Fatalf("CPUTimeMs = %d, want 1100", got.CPUTimeMs)
	}
}

func TestProcessGroupSamplerKeepsPeakObservedUsage(t *testing.T) {
	s := &ProcessGroupSampler{}
	s.record(ResourceUsage{PeakRSSKB: 100, CPUTimeMs: 40})
	s.record(ResourceUsage{PeakRSSKB: 80, CPUTimeMs: 60})

	got := s.snapshot()
	if got.PeakRSSKB != 100 {
		t.Fatalf("PeakRSSKB = %d, want 100", got.PeakRSSKB)
	}
	if got.CPUTimeMs != 60 {
		t.Fatalf("CPUTimeMs = %d, want 60", got.CPUTimeMs)
	}
}

func TestProcessGroupSamplerStopIsIdempotent(t *testing.T) {
	s := StartProcessGroupSampler(-1, time.Hour)
	first := s.Stop()
	second := s.Stop()

	if first != second {
		t.Fatalf("second Stop changed usage: first=%+v second=%+v", first, second)
	}
}

func writeFakeProc(t *testing.T, root string, pid int, stat, status string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0644); err != nil {
		t.Fatal(err)
	}
}

func makeProcStat(pid int, comm string, pgrp int, utime, stime uint64) string {
	fields := []string{
		"S",
		"1",
		strconv.Itoa(pgrp),
		"0",
		"0",
		"0",
		"0",
		"0",
		"0",
		"0",
		"0",
		fmt.Sprintf("%d", utime),
		fmt.Sprintf("%d", stime),
	}
	return fmt.Sprintf("%d (%s) %s\n", pid, comm, strings.Join(fields, " "))
}
