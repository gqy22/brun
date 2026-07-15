package main

import "testing"

func TestParseCPUInfoCountsSocketsAndPhysicalCores(t *testing.T) {
	data := []byte(`processor : 0
physical id : 0
core id : 0
model name : Example CPU

processor : 1
physical id : 0
core id : 0
model name : Example CPU

processor : 2
physical id : 1
core id : 0
model name : Example CPU
`)
	got := parseCPUInfo(data)
	if got.Model != "Example CPU" || got.Sockets != 2 || got.PhysicalCores != 2 || got.LogicalCPUs != 3 {
		t.Fatalf("CPU info = %+v", got)
	}
}

func TestParseMemInfoReturnsBytes(t *testing.T) {
	total, available := parseMemInfo([]byte("MemTotal:       1024 kB\nMemAvailable:    256 kB\n"))
	if total != 1024*1024 || available != 256*1024 {
		t.Fatalf("memory = total %d available %d", total, available)
	}
}

func TestParseLoadAverage(t *testing.T) {
	one, five, fifteen := parseLoadAverage([]byte("1.25 0.75 0.50 2/100 123\n"))
	if one != 1.25 || five != 0.75 || fifteen != 0.50 {
		t.Fatalf("load average = %v %v %v", one, five, fifteen)
	}
}

func TestHostIDDoesNotExposeHostname(t *testing.T) {
	got := anonymizedHostID("private-hostname")
	if got == "" || got == "private-hostname" || len(got) != 12 {
		t.Fatalf("host ID = %q", got)
	}
	if got != anonymizedHostID("private-hostname") {
		t.Fatal("host ID is not stable")
	}
}
