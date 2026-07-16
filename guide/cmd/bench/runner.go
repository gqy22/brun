package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biotools/brun/internal"
	"gopkg.in/yaml.v3"
)

type runOptions struct {
	CaseID          string
	Tier            string
	CaseRoot        string
	DatasetRoot     string
	CacheRoot       string
	WorkingDir      string
	BrunBin         string
	CheckJobs       int
	WarmupsOverride *int
	RepeatsOverride *int
	MatrixOverrides map[string][]string
}

type datasetManifest struct {
	Schema int    `yaml:"schema"`
	ID     string `yaml:"id"`
	Tier   string `yaml:"tier"`
	Source struct {
		Filename string `yaml:"filename"`
		Bytes    int64  `yaml:"bytes"`
	} `yaml:"source"`
	Metadata struct {
		Records int64 `yaml:"records"`
		Samples int   `yaml:"samples"`
		Contigs int   `yaml:"contigs"`
	} `yaml:"metadata"`
}

type checkRecord struct {
	Variant  string
	Check    string
	Status   string
	Value    string
	Expected string
}

func runBenchmark(ctx context.Context, options runOptions) (resultDir string, retErr error) {
	if options.CaseID == "" || options.Tier == "" {
		return "", errors.New("case ID 和 tier 不能为空")
	}
	casePath, err := findYAMLByID(options.CaseRoot, options.CaseID)
	if err != nil {
		return "", err
	}
	item, err := loadBenchmarkCase(casePath)
	if err != nil {
		return "", err
	}
	datasetID, ok := item.Benchmark.Datasets[options.Tier]
	if !ok {
		return "", fmt.Errorf("case %s 没有声明 tier %s 的数据集", item.ID, options.Tier)
	}
	datasetPath, err := findYAMLByID(options.DatasetRoot, datasetID)
	if err != nil {
		return "", err
	}
	dataset, err := loadDatasetManifest(datasetPath)
	if err != nil {
		return "", err
	}
	if dataset.Tier != options.Tier {
		return "", fmt.Errorf("数据集 %s 的 tier 是 %s，不是 %s", dataset.ID, dataset.Tier, options.Tier)
	}

	for _, tool := range item.Requires.Tools {
		if _, err := exec.LookPath(tool); err != nil {
			return "", fmt.Errorf("缺少必需工具 %s: %w", tool, err)
		}
	}
	if _, err := exec.LookPath("/usr/bin/time"); err != nil {
		return "", errors.New("需要 GNU /usr/bin/time")
	}
	brunBin := options.BrunBin
	if brunBin == "" {
		brunBin = "./bin/brun"
	}
	brunBin, err = filepath.Abs(brunBin)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(brunBin); statErr != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("benchmark 需要可执行的 brun 二进制 %s；请先运行 make build", brunBin)
	}

	cacheRoot, err := filepath.Abs(options.CacheRoot)
	if err != nil {
		return "", err
	}
	input := filepath.Join(cacheRoot, "downloads", dataset.ID, dataset.Source.Filename)
	inputInfo, err := os.Stat(input)
	if err != nil {
		return "", fmt.Errorf("缺少数据集文件 %s: %w", input, err)
	}

	warmups := item.Benchmark.Warmups
	repeats := item.Benchmark.Repeats
	if options.WarmupsOverride != nil {
		warmups = *options.WarmupsOverride
	}
	if options.RepeatsOverride != nil {
		repeats = *options.RepeatsOverride
	}
	if warmups < 0 || repeats <= 0 {
		return "", errors.New("warmups 必须非负且 repeats 必须为正整数")
	}
	runID := fmt.Sprintf("%s-%d", time.Now().Format("20060102-150405"), os.Getpid())
	resultDir = filepath.Join(cacheRoot, "benchmarks", item.ID, options.Tier, runID)
	workDir := filepath.Join(cacheRoot, "work", item.ID+"-"+runID)
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", err
	}
	defer os.RemoveAll(workDir)

	workingDir := options.WorkingDir
	if workingDir == "" {
		workingDir, err = os.Getwd()
		if err != nil {
			return resultDir, err
		}
	}

	baseValues := make(map[string]string, len(item.Benchmark.Variables)+5)
	for key, value := range item.Benchmark.Variables {
		baseValues[key] = value
	}
	baseValues["input"] = input
	baseValues["work"] = workDir
	baseValues["cache"] = cacheRoot
	baseValues["dataset"] = dataset.ID
	baseValues["tier"] = options.Tier

	extension := item.Benchmark.OutputExtension
	if extension == "" {
		extension = ".out"
	}
	var setupCommand []string
	if item.Benchmark.Setup != nil {
		setupCommand, err = expandArguments(item.Benchmark.Setup.Command, baseValues)
		if err != nil {
			return resultDir, fmt.Errorf("setup: %w", err)
		}
	}

	variants, err := expandVariants(item.Benchmark.Variants, options.MatrixOverrides)
	if err != nil {
		return resultDir, err
	}
	checkJobs, err := resolveCheckJobs(options.CheckJobs, len(variants))
	if err != nil {
		return resultDir, err
	}
	baselinePresent := false
	for _, variant := range variants {
		if variant.ID == item.Benchmark.Baseline {
			baselinePresent = true
			break
		}
	}
	if !baselinePresent {
		return resultDir, fmt.Errorf("matrix 覆盖后缺少 baseline variant %s", item.Benchmark.Baseline)
	}
	commands := make(map[string][]string, len(variants))
	outputs := make(map[string]string, len(variants))
	for _, variant := range variants {
		output := filepath.Join(workDir, variant.ID+extension)
		values := cloneStrings(baseValues)
		for key, value := range variant.Values {
			values[key] = value
		}
		values["variant"] = variant.ID
		values["output"] = output
		expanded, err := expandArguments(variant.Command, values)
		if err != nil {
			return resultDir, fmt.Errorf("variant %s: %w", variant.ID, err)
		}
		commands[variant.ID] = expanded
		outputs[variant.ID] = output
	}

	versions := collectVersions(ctx, workingDir, item.Benchmark.Versions)
	versions["brun"] = collectVersion(ctx, workingDir, []string{brunBin, "--version"})
	device := collectDeviceInfo(input, resultDir)
	stateStart := collectRuntimeState()
	if err := writeEnvironment(filepath.Join(resultDir, "environment.tsv"), item, dataset, input, inputInfo.Size(), warmups, repeats, checkJobs, versions, device); err != nil {
		return resultDir, err
	}
	statePath := filepath.Join(resultDir, "state.tsv")
	if err := writeState(statePath, stateStart, nil, "running"); err != nil {
		return resultDir, err
	}
	if err := writeCommands(filepath.Join(resultDir, "commands.tsv"), setupCommand, variants, commands); err != nil {
		return resultDir, err
	}
	finalized := false
	finalize := func(status string) error {
		stateEnd := collectRuntimeState()
		if err := writeState(statePath, stateStart, &stateEnd, status); err != nil {
			return err
		}
		return writeManifest(resultDir)
	}
	defer func() {
		if finalized {
			return
		}
		status := "failed"
		if errors.Is(ctx.Err(), context.Canceled) {
			status = "cancelled"
		}
		if err := finalize(status); retErr == nil && err != nil {
			retErr = fmt.Errorf("收尾 benchmark 结果: %w", err)
		}
	}()

	if len(setupCommand) > 0 {
		fmt.Println("[setup]")
		if err := executeSetup(ctx, workingDir, setupCommand); err != nil {
			return resultDir, fmt.Errorf("setup 执行失败: %w", err)
		}
	}

	plan := buildPlan(variants, warmups, repeats)
	runs := make([]runRecord, 0, len(plan))
	runsPath := filepath.Join(resultDir, "runs.tsv")
	if err := writeRuns(runsPath, runs); err != nil {
		return resultDir, err
	}
	for _, planned := range plan {
		output := outputs[planned.Variant.ID]
		if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
			return resultDir, err
		}
		fmt.Printf("[%s %d] %s\n", planned.Phase, planned.Repeat, planned.Variant.ID)
		record, runErr := executeTimed(ctx, workingDir, workDir, brunBin, item.ID, options.Tier, planned, commands[planned.Variant.ID], output)
		if record.Variant != "" {
			runs = append(runs, record)
		}
		if err := writeRuns(runsPath, runs); err != nil {
			return resultDir, err
		}
		if runErr != nil {
			return resultDir, fmt.Errorf("variant %s 执行失败: %w", planned.Variant.ID, runErr)
		}
	}

	checks, err := runChecks(ctx, workingDir, item, variants, baseValues, outputs, checkJobs)
	if writeErr := writeChecks(filepath.Join(resultDir, "checks.tsv"), checks); writeErr != nil {
		return resultDir, writeErr
	}
	if err != nil {
		return resultDir, err
	}

	summaries, err := summarizeRuns(runs, item.Benchmark.Baseline)
	if err != nil {
		return resultDir, err
	}
	if err := writeSummary(filepath.Join(resultDir, "summary.tsv"), summaries); err != nil {
		return resultDir, err
	}
	if err := writeReport(filepath.Join(resultDir, "report.md"), item, dataset, warmups, repeats, checkJobs, summaries, device); err != nil {
		return resultDir, err
	}
	if err := finalize("success"); err != nil {
		return resultDir, err
	}
	finalized = true
	return resultDir, nil
}

func resolveCheckJobs(configured, variants int) (int, error) {
	if configured < 0 {
		return 0, errors.New("check-jobs 必须为非负整数")
	}
	if variants <= 0 {
		return 0, errors.New("正确性检查缺少 variant")
	}
	if configured == 0 || configured > variants {
		return variants, nil
	}
	return configured, nil
}

func findYAMLByID(root, wanted string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var header struct {
			ID string `yaml:"id"`
		}
		if err := yaml.Unmarshal(data, &header); err != nil {
			return fmt.Errorf("解析 %s: %w", path, err)
		}
		if header.ID == wanted {
			if found != "" {
				return fmt.Errorf("ID %s 同时出现在 %s 和 %s", wanted, found, path)
			}
			found = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("在 %s 中找不到 ID %s", root, wanted)
	}
	return found, nil
}

func loadDatasetManifest(path string) (datasetManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return datasetManifest{}, err
	}
	var dataset datasetManifest
	if err := yaml.Unmarshal(data, &dataset); err != nil {
		return datasetManifest{}, fmt.Errorf("解析 %s: %w", path, err)
	}
	if dataset.Schema != 1 || dataset.ID == "" || dataset.Tier == "" || dataset.Source.Filename == "" {
		return datasetManifest{}, fmt.Errorf("%s: 数据集清单缺少 schema=1、id、tier 或 source.filename", path)
	}
	return dataset, nil
}

func executeTimed(ctx context.Context, workingDir, workDir, brunBin, caseID, tier string, planned plannedRun, command []string, output string) (runRecord, error) {
	metricsPath := filepath.Join(workDir, fmt.Sprintf("time-%04d.tsv", planned.Order))
	arguments := []string{"-q", "-o", metricsPath, "-f", "%e\t%U\t%S\t%M\t%x", "--"}
	arguments = append(arguments, command...)
	brunRunID := internal.GenerateRunID()
	brunArgs := []string{
		"run", "--foreground", "--no-fs-diff", "--require-cgroup", "--run-id", brunRunID,
		"--cwd", workingDir, "--name", fmt.Sprintf("bench-%s-%s-%s-%d", caseID, planned.Variant.ID, planned.Phase, planned.Repeat),
		"--project", "brun", "--tag", "guide,benchmark," + tier + "," + caseID, "--", "/usr/bin/time",
	}
	brunArgs = append(brunArgs, arguments...)
	cmd := exec.CommandContext(ctx, brunBin, brunArgs...)
	configureManagedCommand(cmd)
	cmd.Dir = workingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	runErr := cmd.Run()
	brunRun, brunErr := loadBrunRun(brunRunID)
	if brunErr != nil {
		return runRecord{}, fmt.Errorf("读取 brun run %s: %w", brunRunID, brunErr)
	}
	if brunRun.ResourceBackend != "cgroup_v2" || brunRun.ResourceStatus != "ok" {
		return runRecord{}, fmt.Errorf("brun run %s 未获得 cgroup 精确指标: backend=%s status=%s fallback=%s",
			brunRunID, brunRun.ResourceBackend, brunRun.ResourceStatus, brunRun.ResourceFallback)
	}

	data, err := os.ReadFile(metricsPath)
	if err != nil {
		return runRecord{}, fmt.Errorf("读取 time 结果: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(string(data)))
	if len(fields) != 5 {
		return runRecord{}, fmt.Errorf("无法解析 time 结果 %q", strings.TrimSpace(string(data)))
	}
	wall, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return runRecord{}, err
	}
	user, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return runRecord{}, err
	}
	system, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return runRecord{}, err
	}
	rss, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return runRecord{}, err
	}
	exitCode, err := strconv.Atoi(fields[4])
	if err != nil {
		return runRecord{}, err
	}
	var outputBytes int64
	if info, statErr := os.Stat(output); statErr == nil {
		outputBytes = info.Size()
	}
	record := runRecord{
		Variant:           planned.Variant.ID,
		Phase:             planned.Phase,
		Repeat:            planned.Repeat,
		Order:             planned.Order,
		WallSeconds:       wall,
		UserSeconds:       user,
		SystemSeconds:     system,
		MaxRSSKB:          rss,
		ExitCode:          exitCode,
		OutputBytes:       outputBytes,
		BrunRunID:         brunRunID,
		ResourceBackend:   brunRun.ResourceBackend,
		BrunDurationMs:    brunRun.DurationMs,
		CgroupCPUUserMs:   brunRun.CPUUserMs,
		CgroupCPUSystemMs: brunRun.CPUSystemMs,
		MemoryPeakBytes:   brunRun.MemoryPeakBytes,
		IOReadBytes:       brunRun.IOReadBytes,
		IOWriteBytes:      brunRun.IOWriteBytes,
		OOMKillCount:      brunRun.OOMKillCount,
		PIDsPeak:          brunRun.PIDsPeak,
	}
	if runErr != nil {
		return record, runErr
	}
	if exitCode != 0 {
		return record, fmt.Errorf("退出码为 %d", exitCode)
	}
	if brunRun.Status != "success" || brunRun.ExitCode != 0 {
		return record, fmt.Errorf("brun run %s 状态为 %s，退出码 %d", brunRunID, brunRun.Status, brunRun.ExitCode)
	}
	if outputBytes == 0 {
		return record, errors.New("没有生成非空输出")
	}
	return record, nil
}

func loadBrunRun(runID string) (*internal.Run, error) {
	store, err := internal.OpenStoreReadOnly(filepath.Join(internal.HomeDir(), "db.sqlite"))
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.GetRun(runID)
}

func runChecks(ctx context.Context, workingDir string, item benchmarkCase, variants []benchmarkVariant, baseValues map[string]string, outputs map[string]string, jobs int) ([]checkRecord, error) {
	var records []checkRecord
	for _, check := range item.Benchmark.Checks {
		type checkResult struct {
			value string
			err   error
		}
		results := make([]checkResult, len(variants))
		semaphore := make(chan struct{}, min(jobs, len(variants)))
		var group sync.WaitGroup
		for index, variant := range variants {
			index, variant := index, variant
			group.Add(1)
			go func() {
				defer group.Done()
				select {
				case semaphore <- struct{}{}:
					defer func() { <-semaphore }()
				case <-ctx.Done():
					results[index].err = ctx.Err()
					return
				}
				values := cloneStrings(baseValues)
				for key, value := range variant.Values {
					values[key] = value
				}
				values["variant"] = variant.ID
				values["output"] = outputs[variant.ID]
				command, err := expandArguments(check.Command, values)
				if err != nil {
					results[index].err = err
					return
				}
				digest := sha256.New()
				cmd := exec.CommandContext(ctx, command[0], command[1:]...)
				configureManagedCommand(cmd)
				cmd.Dir = workingDir
				cmd.Stdout = digest
				var stderr strings.Builder
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					results[index].value = strings.TrimSpace(stderr.String())
					results[index].err = err
					return
				}
				results[index].value = hex.EncodeToString(digest.Sum(nil))
			}()
		}
		group.Wait()
		valuesByVariant := make(map[string]string, len(variants))
		for index, variant := range variants {
			result := results[index]
			if result.err != nil {
				records = append(records, checkRecord{Variant: variant.ID, Check: check.ID, Status: "error", Value: result.value})
				return records, fmt.Errorf("check %s/%s 执行失败: %w", check.ID, variant.ID, result.err)
			}
			valuesByVariant[variant.ID] = result.value
		}
		expected := valuesByVariant[item.Benchmark.Baseline]
		for _, variant := range variants {
			value := valuesByVariant[variant.ID]
			status := "pass"
			if value != expected {
				status = "fail"
			}
			records = append(records, checkRecord{Variant: variant.ID, Check: check.ID, Status: status, Value: value, Expected: expected})
			if status == "fail" {
				return records, fmt.Errorf("check %s: %s 与 baseline 不一致", check.ID, variant.ID)
			}
		}
	}
	return records, nil
}

func executeSetup(ctx context.Context, workingDir string, command []string) error {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	configureManagedCommand(cmd)
	cmd.Dir = workingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd.Run()
}

func collectVersions(ctx context.Context, workingDir string, commands []versionCommand) map[string]string {
	versions := make(map[string]string, len(commands))
	for _, item := range commands {
		if item.Name == "" || len(item.Command) == 0 {
			continue
		}
		cmd := exec.CommandContext(ctx, item.Command[0], item.Command[1:]...)
		configureManagedCommand(cmd)
		cmd.Dir = workingDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			versions[item.Name] = "unavailable"
			continue
		}
		versions[item.Name] = strings.TrimSpace(string(output))
	}
	return versions
}

func collectVersion(ctx context.Context, workingDir string, command []string) string {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	configureManagedCommand(cmd)
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(output))
}

func writeEnvironment(path string, item benchmarkCase, dataset datasetManifest, input string, inputBytes int64, warmups, repeats, checkJobs int, versions map[string]string, device deviceInfo) error {
	rows := [][]string{
		{"field", "value"},
		{"experiment_id", item.ID},
		{"date", time.Now().Format(time.RFC3339)},
		{"dataset", dataset.ID},
		{"tier", dataset.Tier},
		{"input", input},
		{"input_bytes", strconv.FormatInt(inputBytes, 10)},
		{"records", strconv.FormatInt(dataset.Metadata.Records, 10)},
		{"samples", strconv.Itoa(dataset.Metadata.Samples)},
		{"contigs", strconv.Itoa(dataset.Metadata.Contigs)},
		{"warmups", strconv.Itoa(warmups)},
		{"repeats", strconv.Itoa(repeats)},
		{"check_jobs", strconv.Itoa(checkJobs)},
		{"order", item.Benchmark.Order},
		{"cache_policy", item.Benchmark.CachePolicy},
		{"system", runtime.GOOS + "/" + runtime.GOARCH},
		{"host_id", device.HostID},
		{"kernel", device.Kernel},
		{"cpu_model", device.CPU.Model},
		{"cpu_sockets", intOrUnavailable(device.CPU.Sockets)},
		{"physical_cores", intOrUnavailable(device.CPU.PhysicalCores)},
		{"logical_cpus", intOrUnavailable(device.CPU.LogicalCPUs)},
		{"memory_total_bytes", int64OrUnavailable(device.MemoryTotalBytes)},
		{"cpu_governor", device.CPUGovernor},
		{"input_filesystem_source", device.InputFilesystem.Source},
		{"input_filesystem_type", device.InputFilesystem.Type},
		{"input_filesystem_options", device.InputFilesystem.Options},
		{"input_storage_model", device.InputFilesystem.DeviceModel},
		{"input_storage_rotational", device.InputFilesystem.Rotational},
		{"output_filesystem_source", device.OutputFilesystem.Source},
		{"output_filesystem_type", device.OutputFilesystem.Type},
		{"output_filesystem_options", device.OutputFilesystem.Options},
		{"output_storage_model", device.OutputFilesystem.DeviceModel},
		{"output_storage_rotational", device.OutputFilesystem.Rotational},
	}
	for _, name := range sortedKeys(versions) {
		rows = append(rows, []string{"version." + name, versions[name]})
	}
	return writeTSV(path, rows)
}

func writeState(path string, start runtimeState, end *runtimeState, status string) error {
	endStatus := "pending"
	endTimestamp := "pending"
	endLoadOne := "pending"
	endLoadFive := "pending"
	endLoadFifteen := "pending"
	endMemory := "pending"
	endFrequency := "pending"
	if end != nil {
		endStatus = status
		endTimestamp = end.Timestamp.Format(time.RFC3339Nano)
		endLoadOne = formatStateFloat(end.LoadOne, true)
		endLoadFive = formatStateFloat(end.LoadFive, true)
		endLoadFifteen = formatStateFloat(end.LoadFifteen, true)
		endMemory = int64OrUnavailable(end.MemoryAvailableBytes)
		endFrequency = formatStateFloat(end.CPUFrequencyMHz, false)
	}
	return writeTSV(path, [][]string{
		{"metric", "start", "end"},
		{"status", "running", endStatus},
		{"timestamp", start.Timestamp.Format(time.RFC3339Nano), endTimestamp},
		{"load_1m", formatStateFloat(start.LoadOne, true), endLoadOne},
		{"load_5m", formatStateFloat(start.LoadFive, true), endLoadFive},
		{"load_15m", formatStateFloat(start.LoadFifteen, true), endLoadFifteen},
		{"memory_available_bytes", int64OrUnavailable(start.MemoryAvailableBytes), endMemory},
		{"cpu_frequency_mhz", formatStateFloat(start.CPUFrequencyMHz, false), endFrequency},
	})
}

func writeCommands(path string, setup []string, variants []benchmarkVariant, commands map[string][]string) error {
	rows := [][]string{{"variant", "command"}}
	if len(setup) > 0 {
		rows = append(rows, []string{"@setup", displayCommand(setup)})
	}
	for _, variant := range variants {
		rows = append(rows, []string{variant.ID, displayCommand(commands[variant.ID])})
	}
	return writeTSV(path, rows)
}

func writeRuns(path string, runs []runRecord) error {
	rows := [][]string{{"variant", "phase", "repeat", "run_order", "brun_run_id", "resource_backend", "wall_seconds", "user_seconds", "system_seconds", "max_rss_kb", "brun_duration_ms", "cgroup_cpu_user_ms", "cgroup_cpu_system_ms", "memory_peak_bytes", "io_read_bytes", "io_write_bytes", "oom_kill_count", "pids_peak", "exit_code", "output_bytes"}}
	for _, run := range runs {
		rows = append(rows, []string{
			run.Variant, run.Phase, strconv.Itoa(run.Repeat), strconv.Itoa(run.Order), run.BrunRunID, run.ResourceBackend,
			formatFloat(run.WallSeconds), formatFloat(run.UserSeconds), formatFloat(run.SystemSeconds),
			strconv.FormatInt(run.MaxRSSKB, 10), strconv.FormatInt(run.BrunDurationMs, 10),
			strconv.FormatInt(run.CgroupCPUUserMs, 10), strconv.FormatInt(run.CgroupCPUSystemMs, 10),
			strconv.FormatInt(run.MemoryPeakBytes, 10), strconv.FormatInt(run.IOReadBytes, 10),
			strconv.FormatInt(run.IOWriteBytes, 10), strconv.FormatInt(run.OOMKillCount, 10), strconv.FormatInt(run.PIDsPeak, 10),
			strconv.Itoa(run.ExitCode), strconv.FormatInt(run.OutputBytes, 10),
		})
	}
	return writeTSV(path, rows)
}

func writeChecks(path string, checks []checkRecord) error {
	rows := [][]string{{"variant", "check", "status", "value", "expected"}}
	for _, check := range checks {
		rows = append(rows, []string{check.Variant, check.Check, check.Status, check.Value, check.Expected})
	}
	return writeTSV(path, rows)
}

func writeSummary(path string, summaries []summaryRecord) error {
	rows := [][]string{{"variant", "runs", "mean_wall_seconds", "median_wall_seconds", "min_wall_seconds", "max_wall_seconds", "stddev_wall_seconds", "cv_percent", "mean_cpu_seconds", "mean_max_rss_kb", "average_cores", "mean_cgroup_cpu_seconds", "mean_memory_peak_bytes", "mean_io_read_bytes", "mean_io_write_bytes", "cgroup_average_cores", "speedup_vs_baseline"}}
	for _, item := range summaries {
		rows = append(rows, []string{
			item.Variant, strconv.Itoa(item.Runs), formatFloat(item.MeanWallSeconds), formatFloat(item.MedianWallSeconds),
			formatFloat(item.MinWallSeconds), formatFloat(item.MaxWallSeconds), formatFloat(item.StddevWallSeconds),
			formatFloat(item.CVPercent), formatFloat(item.MeanCPUSeconds), formatFloat(item.MeanMaxRSSKB),
			formatFloat(item.AverageCores), formatFloat(item.MeanCgroupCPUSeconds), formatFloat(item.MeanMemoryPeakBytes),
			formatFloat(item.MeanIOReadBytes), formatFloat(item.MeanIOWriteBytes), formatFloat(item.CgroupAverageCores), formatFloat(item.SpeedupVsBaseline),
		})
	}
	return writeTSV(path, rows)
}

func writeReport(path string, item benchmarkCase, dataset datasetManifest, warmups, repeats, checkJobs int, summaries []summaryRecord, device deviceInfo) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	fmt.Fprintf(file, "# Benchmark draft: %s\n\n", item.ID)
	fmt.Fprintln(file, "本文件由统一 benchmark runner 自动生成，需人工审阅后才能作为正式经验结论。")
	fmt.Fprintf(file, "\n- Dataset: `%s` (%s)\n- CPU: `%s`\n- Logical CPUs: %d\n- Memory: %s bytes\n- Input filesystem: `%s`\n- Output filesystem: `%s`\n- Cache policy: `%s`\n- Warmups: %d\n- Repeats: %d\n- Check jobs: %d\n- Order: `%s`\n\n",
		dataset.ID, dataset.Tier, device.CPU.Model, device.CPU.LogicalCPUs, int64OrUnavailable(device.MemoryTotalBytes),
		device.InputFilesystem.Type, device.OutputFilesystem.Type, item.Benchmark.CachePolicy, warmups, repeats, checkJobs, item.Benchmark.Order)
	fmt.Fprintln(file, "| Variant | Median wall (s) | cgroup CPU (s) | Memory peak (bytes) | I/O read | I/O write | Average cores | Speedup | CV |")
	fmt.Fprintln(file, "|---|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, summary := range summaries {
		fmt.Fprintf(file, "| %s | %.3f | %.3f | %.0f | %.0f | %.0f | %.3f | %.3f× | %.2f%% |\n",
			summary.Variant, summary.MedianWallSeconds, summary.MeanCgroupCPUSeconds, summary.MeanMemoryPeakBytes,
			summary.MeanIOReadBytes, summary.MeanIOWriteBytes, summary.CgroupAverageCores, summary.SpeedupVsBaseline, summary.CVPercent)
	}
	return file.Sync()
}

func writeManifest(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "manifest.sha256" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	file, err := os.Create(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		return err
	}
	defer file.Close()
	for _, name := range names {
		digest, err := checksumFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(file, "%s  %s\n", digest, name)
	}
	return file.Sync()
}

func checksumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeTSV(path string, rows [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	buffer := bufio.NewWriter(file)
	writer := csv.NewWriter(buffer)
	writer.Comma = '\t'
	writer.UseCRLF = false
	cleaned := make([][]string, len(rows))
	for i, row := range rows {
		cleaned[i] = make([]string, len(row))
		for j, value := range row {
			value = strings.ReplaceAll(value, "\r\n", "\n")
			value = strings.ReplaceAll(value, "\r", "\n")
			cleaned[i][j] = strings.ReplaceAll(value, "\n", `\n`)
		}
	}
	if err := writer.WriteAll(cleaned); err != nil {
		return err
	}
	if err := buffer.Flush(); err != nil {
		return err
	}
	return file.Sync()
}

func displayCommand(arguments []string) string {
	quoted := make([]string, len(arguments))
	for i, argument := range arguments {
		if argument != "" && strings.IndexFunc(argument, func(r rune) bool {
			return !(r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '=' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
		}) == -1 {
			quoted[i] = argument
			continue
		}
		quoted[i] = "'" + strings.ReplaceAll(argument, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}

func cloneStrings(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values)+2)
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func intOrUnavailable(value int) string {
	if value <= 0 {
		return "unavailable"
	}
	return strconv.Itoa(value)
}

func int64OrUnavailable(value int64) string {
	if value <= 0 {
		return "unavailable"
	}
	return strconv.FormatInt(value, 10)
}

func formatStateFloat(value float64, zeroValid bool) string {
	if value < 0 || value == 0 && !zeroValid {
		return "unavailable"
	}
	return strconv.FormatFloat(value, 'f', 3, 64)
}
