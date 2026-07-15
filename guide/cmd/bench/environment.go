package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type cpuDetails struct {
	Model         string
	Sockets       int
	PhysicalCores int
	LogicalCPUs   int
}

type filesystemDetails struct {
	Source      string
	Type        string
	Options     string
	DeviceModel string
	Rotational  string
}

type deviceInfo struct {
	HostID           string
	Kernel           string
	CPU              cpuDetails
	MemoryTotalBytes int64
	CPUGovernor      string
	InputFilesystem  filesystemDetails
	OutputFilesystem filesystemDetails
}

type runtimeState struct {
	Timestamp            time.Time
	LoadOne              float64
	LoadFive             float64
	LoadFifteen          float64
	MemoryAvailableBytes int64
	CPUFrequencyMHz      float64
}

func collectDeviceInfo(inputPath, outputPath string) deviceInfo {
	info := deviceInfo{
		HostID:           "unavailable",
		Kernel:           runtime.GOOS + "/" + runtime.GOARCH,
		CPUGovernor:      "unavailable",
		InputFilesystem:  collectFilesystem(inputPath),
		OutputFilesystem: collectFilesystem(outputPath),
	}
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		info.HostID = anonymizedHostID(hostname)
	}
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		info.CPU = parseCPUInfo(data)
	}
	if info.CPU.LogicalCPUs == 0 {
		info.CPU.LogicalCPUs = runtime.NumCPU()
	}
	if info.CPU.Model == "" {
		info.CPU.Model = "unavailable"
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		info.MemoryTotalBytes, _ = parseMemInfo(data)
	}
	if release, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		info.Kernel = runtime.GOOS + " " + strings.TrimSpace(string(release)) + " " + runtime.GOARCH
	}
	if governor, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor"); err == nil {
		if value := strings.TrimSpace(string(governor)); value != "" {
			info.CPUGovernor = value
		}
	}
	return info
}

func collectRuntimeState() runtimeState {
	state := runtimeState{Timestamp: time.Now()}
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		state.LoadOne, state.LoadFive, state.LoadFifteen = parseLoadAverage(data)
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		_, state.MemoryAvailableBytes = parseMemInfo(data)
	}
	if data, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq"); err == nil {
		frequencyKHz, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		state.CPUFrequencyMHz = frequencyKHz / 1000
	}
	return state
}

func parseCPUInfo(data []byte) cpuDetails {
	var result cpuDetails
	sockets := make(map[string]bool)
	cores := make(map[string]bool)
	for _, block := range strings.Split(strings.TrimSpace(string(data)), "\n\n") {
		var physicalID, coreID string
		processor := false
		for _, line := range strings.Split(block, "\n") {
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			switch key {
			case "processor":
				processor = true
			case "model name", "Hardware":
				if result.Model == "" {
					result.Model = value
				}
			case "physical id":
				physicalID = value
			case "core id":
				coreID = value
			}
		}
		if processor {
			result.LogicalCPUs++
		}
		if physicalID != "" {
			sockets[physicalID] = true
		}
		if physicalID != "" && coreID != "" {
			cores[physicalID+":"+coreID] = true
		}
	}
	result.Sockets = len(sockets)
	result.PhysicalCores = len(cores)
	return result
}

func parseMemInfo(data []byte) (totalBytes, availableBytes int64) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			totalBytes = value * 1024
		case "MemAvailable":
			availableBytes = value * 1024
		}
	}
	return totalBytes, availableBytes
}

func parseLoadAverage(data []byte) (one, five, fifteen float64) {
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	one, _ = strconv.ParseFloat(fields[0], 64)
	five, _ = strconv.ParseFloat(fields[1], 64)
	fifteen, _ = strconv.ParseFloat(fields[2], 64)
	return one, five, fifteen
}

func anonymizedHostID(hostname string) string {
	digest := sha256.Sum256([]byte(hostname))
	return hex.EncodeToString(digest[:])[:12]
}

func collectFilesystem(path string) filesystemDetails {
	details := filesystemDetails{
		Source:      "unavailable",
		Type:        "unavailable",
		Options:     "unavailable",
		DeviceModel: "unavailable",
		Rotational:  "unavailable",
	}
	output, err := exec.Command("findmnt", "-no", "SOURCE,FSTYPE,OPTIONS", "-T", path).Output()
	if err != nil {
		return details
	}
	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) >= 1 {
		details.Source = fields[0]
	}
	if len(fields) >= 2 {
		details.Type = fields[1]
	}
	if len(fields) >= 3 {
		details.Options = strings.Join(fields[2:], " ")
	}
	if strings.HasPrefix(details.Source, "/dev/") {
		if block, err := exec.Command("lsblk", "-ndo", "MODEL,ROTA", details.Source).Output(); err == nil {
			blockFields := strings.Fields(strings.TrimSpace(string(block)))
			if len(blockFields) > 0 {
				details.Rotational = blockFields[len(blockFields)-1]
				if len(blockFields) > 1 {
					details.DeviceModel = strings.Join(blockFields[:len(blockFields)-1], " ")
				}
			}
		}
	}
	return details
}
