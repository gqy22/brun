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
	writeFakeProc(t, root, 101, makeProcStat(101, "bash script", 777, 30, 10), "VmRSS:\t1200 kB\n", "")
	writeFakeProc(t, root, 102, makeProcStat(102, "python worker", 777, 50, 20), "VmRSS:\t3000 kB\n", "")
	writeFakeProc(t, root, 201, makeProcStat(201, "other", 999, 100, 100), "VmRSS:\t9999 kB\n", "")
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

func TestListProcessTreeFromProcIncludesDescendantsAcrossGroups(t *testing.T) {
	root := t.TempDir()
	writeFakeProc(t, root, 101, makeProcStatFull(101, "bash root", 1, 101, 30, 10), "VmRSS:\t1200 kB\n", "bash\x00root\x00")
	writeFakeProc(t, root, 102, makeProcStatFull(102, "worker a", 101, 202, 50, 20), "VmRSS:\t3000 kB\n", "worker-a\x00")
	writeFakeProc(t, root, 103, makeProcStatFull(103, "worker b", 101, 303, 70, 30), "VmRSS:\t5000 kB\n", "worker-b\x00")
	writeFakeChildren(t, root, 101, "102 103\n")

	got := listProcessTreeFromProc(root, 101, 100)
	if len(got) != 3 {
		t.Fatalf("len(ListProcessTree) = %d, want 3", len(got))
	}
	if got[0].PID != 101 || got[1].PID != 102 || got[2].PID != 103 {
		t.Fatalf("unexpected pid order: %+v", got)
	}
	if got[1].Cmdline != "worker-a" {
		t.Fatalf("cmdline = %q, want worker-a", got[1].Cmdline)
	}
}

func TestSampleProcessTreeFromProcAggregatesDescendants(t *testing.T) {
	root := t.TempDir()
	writeFakeProc(t, root, 101, makeProcStatFull(101, "bash root", 1, 101, 30, 10), "VmRSS:\t1200 kB\n", "")
	writeFakeProc(t, root, 102, makeProcStatFull(102, "worker a", 101, 202, 50, 20), "VmRSS:\t3000 kB\n", "")
	writeFakeProc(t, root, 103, makeProcStatFull(103, "worker b", 101, 303, 70, 30), "VmRSS:\t5000 kB\n", "")
	writeFakeChildren(t, root, 101, "102 103\n")

	got := sampleProcessTreeFromProc(root, 101, 100)
	if got.PeakRSSKB != 9200 {
		t.Fatalf("PeakRSSKB = %d, want 9200", got.PeakRSSKB)
	}
	if got.CPUTimeMs != 2100 {
		t.Fatalf("CPUTimeMs = %d, want 2100", got.CPUTimeMs)
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

func writeFakeProc(t *testing.T, root string, pid int, stat, status, cmdline string) {
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
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0644); err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Join(dir, "task", strconv.Itoa(pid))
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "children"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeFakeChildren(t *testing.T, root string, pid int, children string) {
	t.Helper()
	taskDir := filepath.Join(root, strconv.Itoa(pid), "task", strconv.Itoa(pid))
	if err := os.WriteFile(filepath.Join(taskDir, "children"), []byte(children), 0644); err != nil {
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

func makeProcStatFull(pid int, comm string, ppid, pgrp int, utime, stime uint64) string {
	fields := []string{
		"S",
		strconv.Itoa(ppid),
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
