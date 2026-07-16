package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRunChecksKeepsDeterministicOrderWithWorkers(t *testing.T) {
	root := t.TempDir()
	variants := []benchmarkVariant{{ID: "baseline"}, {ID: "second"}, {ID: "third"}}
	outputs := make(map[string]string, len(variants))
	for _, variant := range variants {
		path := filepath.Join(root, variant.ID+".txt")
		if err := os.WriteFile(path, []byte("same\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		outputs[variant.ID] = path
	}
	item := benchmarkCase{}
	item.Benchmark.Baseline = "baseline"
	item.Benchmark.Checks = []benchmarkCheck{{ID: "content", Command: []string{"cat", "{output}"}}}
	records, err := runChecks(context.Background(), root, item, variants, nil, outputs, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %+v", records)
	}
	for index, variant := range variants {
		if records[index].Variant != variant.ID || records[index].Status != "pass" {
			t.Fatalf("records = %+v", records)
		}
	}
}

func TestResolveCheckJobsDefaultsToVariantCount(t *testing.T) {
	got, err := resolveCheckJobs(0, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got != 6 {
		t.Fatalf("jobs = %d, want 6", got)
	}
	got, err = resolveCheckJobs(2, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("jobs = %d, want 2", got)
	}
	got, err = resolveCheckJobs(20, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got != 6 {
		t.Fatalf("jobs = %d, want 6", got)
	}
	if _, err := resolveCheckJobs(-1, 6); err == nil {
		t.Fatal("negative check jobs was accepted")
	}
}

func TestLoadBenchmarkCaseParsesStrictSchema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "case.yaml")
	document := `
schema: 1
id: example.benchmark
guide: example.guide
datasets: [example-smoke]
requires:
  tools: [/usr/bin/time, example]
assertions: [commands_succeed]
benchmark:
  baseline: baseline
  datasets:
    smoke: example-smoke
  warmups: 1
  repeats: 3
  order: balanced
  cache_policy: uncontrolled
  output_extension: .txt
  variables:
    answer: "42"
  versions:
    - name: example
      command: [example, --version]
  variants:
    - id: baseline
      command: [example, --output, "{output}", "{input}"]
    - id: optimized
      command: [example, --fast, "{answer}", --output, "{output}", "{input}"]
  checks:
    - id: decoded
      type: stdout-sha256-equal
      command: [example, decode, "{output}"]
`
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := loadBenchmarkCase(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "example.benchmark" || got.Benchmark.Baseline != "baseline" {
		t.Fatalf("case = %+v", got)
	}
	if got.Benchmark.Datasets["smoke"] != "example-smoke" || len(got.Benchmark.Variants) != 2 {
		t.Fatalf("benchmark = %+v", got.Benchmark)
	}
	if got.Benchmark.Checks[0].Type != "stdout-sha256-equal" {
		t.Fatalf("checks = %+v", got.Benchmark.Checks)
	}
}

func TestLoadBenchmarkCaseRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case.yaml")
	if err := os.WriteFile(path, []byte("schema: 1\nid: example\nunknown: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBenchmarkCase(path); err == nil {
		t.Fatal("loadBenchmarkCase accepted an unknown field")
	}
}

func TestBuildPlanUsesBalancedMeasuredOrder(t *testing.T) {
	variants := []benchmarkVariant{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	plan := buildPlan(variants, 1, 3)

	var measured []string
	for _, run := range plan {
		if run.Phase == "measured" {
			measured = append(measured, run.Variant.ID)
		}
	}
	want := []string{"a", "b", "c", "b", "c", "a", "c", "a", "b"}
	if !reflect.DeepEqual(measured, want) {
		t.Fatalf("measured order = %v, want %v", measured, want)
	}
}

func TestExpandVariantsBuildsDeterministicMatrixIDs(t *testing.T) {
	variants := []benchmarkVariant{
		{
			ID:      "standard",
			Matrix:  map[string][]string{"threads": {"1", "2", "4"}},
			Command: []string{"tool", "--threads", "{threads}"},
		},
		{ID: "naive", Command: []string{"tool", "--naive"}},
	}

	got, err := expandVariants(variants, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, variant := range got {
		ids = append(ids, variant.ID)
	}
	want := []string{"standard-threads-1", "standard-threads-2", "standard-threads-4", "naive"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("variant IDs = %v, want %v", ids, want)
	}
	if got[2].Values["threads"] != "4" {
		t.Fatalf("matrix values = %+v", got[2].Values)
	}
}

func TestExpandVariantsAppliesMatrixOverride(t *testing.T) {
	variants := []benchmarkVariant{{
		ID:      "standard",
		Matrix:  map[string][]string{"threads": {"1", "2", "4"}},
		Command: []string{"tool", "{threads}"},
	}}
	got, err := expandVariants(variants, map[string][]string{"threads": {"1", "8"}})
	if err != nil {
		t.Fatal(err)
	}
	if ids := []string{got[0].ID, got[1].ID}; !reflect.DeepEqual(ids, []string{"standard-threads-1", "standard-threads-8"}) {
		t.Fatalf("override IDs = %v", ids)
	}
	if _, err := expandVariants(variants, map[string][]string{"compression": {"1"}}); err == nil {
		t.Fatal("expandVariants accepted an unknown matrix override")
	}
}

func TestExpandArgumentsUsesOnlyDeclaredPlaceholders(t *testing.T) {
	args := []string{"tool", "--input", "{input}", "--output={output}", "{threads}"}
	got, err := expandArguments(args, map[string]string{
		"input":   "/data/in.vcf.gz",
		"output":  "/work/out.vcf.gz",
		"threads": "4",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tool", "--input", "/data/in.vcf.gz", "--output=/work/out.vcf.gz", "4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments = %v, want %v", got, want)
	}
	if _, err := expandArguments([]string{"{missing}"}, map[string]string{}); err == nil {
		t.Fatal("expandArguments accepted an undeclared placeholder")
	}
}

func TestSummarizeIgnoresWarmupsAndUsesMedian(t *testing.T) {
	runs := []runRecord{
		{Variant: "base", Phase: "warmup", WallSeconds: 100},
		{Variant: "base", Phase: "measured", WallSeconds: 5, UserSeconds: 4, MaxRSSKB: 100, CgroupCPUUserMs: 4000, MemoryPeakBytes: 1000, IOReadBytes: 10, IOWriteBytes: 20},
		{Variant: "base", Phase: "measured", WallSeconds: 3, UserSeconds: 2, MaxRSSKB: 120},
		{Variant: "base", Phase: "measured", WallSeconds: 4, UserSeconds: 3, MaxRSSKB: 110},
		{Variant: "fast", Phase: "measured", WallSeconds: 2, UserSeconds: 1, MaxRSSKB: 90},
		{Variant: "fast", Phase: "measured", WallSeconds: 1, UserSeconds: 1, MaxRSSKB: 80},
		{Variant: "fast", Phase: "measured", WallSeconds: 3, UserSeconds: 2, MaxRSSKB: 85},
	}

	got, err := summarizeRuns(runs, "base")
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]summaryRecord, len(got))
	for _, item := range got {
		byID[item.Variant] = item
	}
	if byID["base"].MedianWallSeconds != 4 || byID["base"].Runs != 3 {
		t.Fatalf("base summary = %+v", byID["base"])
	}
	if byID["fast"].MedianWallSeconds != 2 || byID["fast"].SpeedupVsBaseline != 2 {
		t.Fatalf("fast summary = %+v", byID["fast"])
	}
	if byID["base"].MeanCgroupCPUSeconds <= 0 || byID["base"].MeanMemoryPeakBytes <= 0 {
		t.Fatalf("base cgroup summary = %+v", byID["base"])
	}
}
