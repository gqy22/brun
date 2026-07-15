package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunBenchmarkWritesStandardArtifacts(t *testing.T) {
	if os.Getenv("BRUN_BENCH_INTEGRATION") != "1" {
		t.Skip("set BRUN_BENCH_INTEGRATION=1 to execute external timing commands")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	caseRoot := filepath.Join(root, "cases")
	datasetRoot := filepath.Join(root, "datasets")
	if err := os.MkdirAll(caseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(datasetRoot, "smoke"), 0o755); err != nil {
		t.Fatal(err)
	}

	datasetDocument := `
schema: 1
id: example-smoke
tier: smoke
source:
  filename: input.txt
  bytes: 5
metadata:
  records: 1
  samples: 0
  contigs: 1
`
	if err := os.WriteFile(filepath.Join(datasetRoot, "smoke", "example.yaml"), []byte(datasetDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	inputDir := filepath.Join(cache, "downloads", "example-smoke")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "input.txt"), []byte("input"), 0o644); err != nil {
		t.Fatal(err)
	}

	caseDocument := `
schema: 1
id: example.benchmark
guide: example.guide
datasets: [example-smoke]
requires:
  tools: [/usr/bin/time, sh, cat]
assertions: [commands_succeed]
benchmark:
  baseline: baseline-mode-a
  datasets:
    smoke: example-smoke
  setup:
    command: [sh, -c, "printf ready > \"$1/setup.txt\"", _, "{cache}"]
  warmups: 1
  repeats: 1
  order: balanced
  cache_policy: uncontrolled
  output_extension: .txt
  versions:
    - name: example
      command: [sh, -c, "printf example-1.0"]
  variants:
    - id: baseline
      matrix:
        mode: [a]
      command: [sh, -c, "sleep 0.02; printf same > \"$1\"", _, "{output}"]
    - id: optimized
      command: [sh, -c, "sleep 0.02; printf same > \"$1\"", _, "{output}"]
  checks:
    - id: content
      type: stdout-sha256-equal
      command: [cat, "{output}"]
`
	if err := os.WriteFile(filepath.Join(caseRoot, "example.yaml"), []byte(caseDocument), 0o644); err != nil {
		t.Fatal(err)
	}

	resultDir, err := runBenchmark(context.Background(), runOptions{
		CaseID:      "example.benchmark",
		Tier:        "smoke",
		CaseRoot:    caseRoot,
		DatasetRoot: datasetRoot,
		CacheRoot:   cache,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"environment.tsv",
		"commands.tsv",
		"runs.tsv",
		"checks.tsv",
		"summary.tsv",
		"state.tsv",
		"report.md",
		"manifest.sha256",
	} {
		info, err := os.Stat(filepath.Join(resultDir, name))
		if err != nil || info.Size() == 0 {
			t.Errorf("artifact %s missing or empty: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(cache, "setup.txt")); err != nil {
		t.Fatalf("setup command did not run: %v", err)
	}

	runs := readTestFile(t, filepath.Join(resultDir, "runs.tsv"))
	if got := strings.Count(strings.TrimSpace(runs), "\n"); got != 4 {
		t.Fatalf("runs.tsv rows = %d, want header + 4 runs\n%s", got+1, runs)
	}
	if !strings.Contains(runs, "warmup") || !strings.Contains(runs, "measured") {
		t.Fatalf("runs.tsv does not preserve phases:\n%s", runs)
	}

	checks := readTestFile(t, filepath.Join(resultDir, "checks.tsv"))
	if !strings.Contains(checks, "baseline-mode-a\tcontent\tpass") || !strings.Contains(checks, "optimized\tcontent\tpass") {
		t.Fatalf("checks.tsv =\n%s", checks)
	}
	summary := readTestFile(t, filepath.Join(resultDir, "summary.tsv"))
	if !strings.Contains(summary, "speedup_vs_baseline") || !strings.Contains(summary, "optimized") {
		t.Fatalf("summary.tsv =\n%s", summary)
	}
	environment := readTestFile(t, filepath.Join(resultDir, "environment.tsv"))
	for _, field := range []string{"cpu_model\t", "memory_total_bytes\t", "input_filesystem_type\t", "cache_policy\tuncontrolled"} {
		if !strings.Contains(environment, field) {
			t.Errorf("environment.tsv missing %q:\n%s", field, environment)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(environment), "\n") {
		if strings.Count(line, "\t") != 1 {
			t.Fatalf("environment.tsv contains a non-tabular physical line %q", line)
		}
	}
	state := readTestFile(t, filepath.Join(resultDir, "state.tsv"))
	if strings.Contains(state, "pending") || !strings.Contains(state, "memory_available_bytes") {
		t.Fatalf("state.tsv is incomplete:\n%s", state)
	}
}

func TestRunBenchmarkCancellationFinalizesPartialResult(t *testing.T) {
	if os.Getenv("BRUN_BENCH_INTEGRATION") != "1" {
		t.Skip("set BRUN_BENCH_INTEGRATION=1 to execute external timing commands")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	caseRoot := filepath.Join(root, "cases")
	datasetRoot := filepath.Join(root, "datasets")
	if err := os.MkdirAll(caseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(datasetRoot, "smoke"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(datasetRoot, "smoke", "example.yaml"), `
schema: 1
id: cancel-smoke
tier: smoke
source:
  filename: input.txt
  bytes: 5
metadata:
  records: 1
  samples: 0
  contigs: 1
`)
	inputDir := filepath.Join(cache, "downloads", "cancel-smoke")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(inputDir, "input.txt"), "input")
	marker := filepath.Join(cache, "child.pid")
	writeTestFile(t, filepath.Join(caseRoot, "cancel.yaml"), `
schema: 1
id: cancel.benchmark
guide: example.guide
datasets: [cancel-smoke]
requires:
  tools: [/usr/bin/time, sh, sleep]
assertions: [commands_succeed]
benchmark:
  baseline: baseline
  datasets:
    smoke: cancel-smoke
  warmups: 0
  repeats: 1
  order: balanced
  cache_policy: uncontrolled
  output_extension: .txt
  variables:
    marker: "`+marker+`"
  variants:
    - id: baseline
      command: [sh, -c, "printf '%s' \"$$\" > \"$1\"; sleep 30; printf same > \"$2\"", _, "{marker}", "{output}"]
    - id: optimized
      command: [sh, -c, "printf same > \"$1\"", _, "{output}"]
`)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var resultDir string
	var runErr error
	go func() {
		defer close(done)
		resultDir, runErr = runBenchmark(ctx, runOptions{
			CaseID:      "cancel.benchmark",
			Tier:        "smoke",
			CaseRoot:    caseRoot,
			DatasetRoot: datasetRoot,
			CacheRoot:   cache,
		})
	}()
	waitForTestFile(t, marker, 10*time.Second)
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("benchmark did not stop after cancellation")
	}
	if runErr == nil || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("runBenchmark error = %v, context = %v", runErr, ctx.Err())
	}
	state := readTestFile(t, filepath.Join(resultDir, "state.tsv"))
	if !strings.Contains(state, "status\trunning\tcancelled") || strings.Contains(state, "pending") {
		t.Fatalf("state.tsv was not finalized as cancelled:\n%s", state)
	}
	if info, err := os.Stat(filepath.Join(resultDir, "manifest.sha256")); err != nil || info.Size() == 0 {
		t.Fatalf("partial manifest missing: %v", err)
	}
	workMatches, err := filepath.Glob(filepath.Join(cache, "work", "cancel.benchmark-*"))
	if err != nil || len(workMatches) != 0 {
		t.Fatalf("work directory was not cleaned: %v, %v", workMatches, err)
	}
	pidText := readTestFile(t, marker)
	pid, err := strconv.Atoi(strings.TrimSpace(pidText))
	if err != nil {
		t.Fatalf("invalid child PID %q: %v", pidText, err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("benchmark child process %d is still alive: %v", pid, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitForTestFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
