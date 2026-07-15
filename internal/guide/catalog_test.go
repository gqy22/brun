package guide

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type catalogCase struct {
	Schema    int      `yaml:"schema"`
	ID        string   `yaml:"id"`
	Guide     string   `yaml:"guide"`
	Datasets  []string `yaml:"datasets"`
	Script    string   `yaml:"script"`
	Benchmark *struct {
		Baseline string            `yaml:"baseline"`
		Datasets map[string]string `yaml:"datasets"`
		Variants []struct {
			ID      string   `yaml:"id"`
			Command []string `yaml:"command"`
		} `yaml:"variants"`
	} `yaml:"benchmark"`
	Requires struct {
		Tools []string `yaml:"tools"`
	} `yaml:"requires"`
	Assertions []string `yaml:"assertions"`
}

type datasetReference struct {
	Schema int    `yaml:"schema"`
	ID     string `yaml:"id"`
	UsedBy struct {
		Guides []string `yaml:"guides"`
		Cases  []string `yaml:"cases"`
	} `yaml:"used_by"`
}

type benchmarkReport struct {
	Schema   int               `yaml:"schema"`
	ID       string            `yaml:"id"`
	Guide    string            `yaml:"guide"`
	Case     string            `yaml:"case"`
	Date     string            `yaml:"date"`
	Summary  string            `yaml:"summary"`
	Tools    map[string]string `yaml:"tools"`
	Datasets []struct {
		ID                   string  `yaml:"id"`
		Role                 string  `yaml:"role"`
		Repeats              int     `yaml:"repeats"`
		Warmups              int     `yaml:"warmups"`
		BaselineWallSeconds  float64 `yaml:"baseline_wall_seconds"`
		OptimizedWallSeconds float64 `yaml:"optimized_wall_seconds"`
		Speedup              float64 `yaml:"speedup"`
	} `yaml:"datasets"`
}

func TestCatalogReferencesAreConsistent(t *testing.T) {
	root := guideDevelopmentRoot(t)
	entries, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	guides := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		guides[entry.ID] = entry
	}

	datasets := loadCatalogFiles[datasetReference](t, filepath.Join(root, "datasets"))
	cases := loadCatalogFiles[catalogCase](t, filepath.Join(root, "cases"))
	reports := loadCatalogFiles[benchmarkReport](t, filepath.Join(root, "reports"))

	datasetByID := indexCatalog(t, datasets, func(item datasetReference) string { return item.ID })
	caseByID := indexCatalog(t, cases, func(item catalogCase) string { return item.ID })
	reportByID := indexCatalog(t, reports, func(item benchmarkReport) string { return item.ID })

	referencedCases := make(map[string]bool)
	referencedReports := make(map[string]bool)
	for _, entry := range entries {
		for _, id := range entry.Evidence.Validations {
			validationCase, ok := caseByID[id]
			if !ok {
				t.Errorf("guide %s references missing validation case %s", entry.ID, id)
				continue
			}
			if validationCase.Guide != entry.ID {
				t.Errorf("validation case %s belongs to %s, referenced by %s", id, validationCase.Guide, entry.ID)
			}
			referencedCases[id] = true
		}
		for _, id := range entry.Evidence.Benchmarks {
			report, ok := reportByID[id]
			if !ok {
				t.Errorf("guide %s references missing benchmark report %s", entry.ID, id)
				continue
			}
			if report.Guide != entry.ID {
				t.Errorf("benchmark report %s belongs to %s, referenced by %s", id, report.Guide, entry.ID)
			}
			referencedReports[id] = true
		}
	}

	for _, item := range cases {
		if item.Schema != 1 {
			t.Errorf("case %s has unsupported schema %d", item.ID, item.Schema)
		}
		if !stableIDPattern.MatchString(item.ID) {
			t.Errorf("case has invalid ID %q", item.ID)
		}
		if _, ok := guides[item.Guide]; !ok {
			t.Errorf("case %s references missing guide %s", item.ID, item.Guide)
		}
		if len(item.Datasets) == 0 || len(item.Requires.Tools) == 0 || len(item.Assertions) == 0 {
			t.Errorf("case %s must declare datasets, required tools and assertions", item.ID)
		}
		for _, id := range item.Datasets {
			if _, ok := datasetByID[id]; !ok {
				t.Errorf("case %s references missing dataset %s", item.ID, id)
			}
		}
		if item.Benchmark != nil {
			if item.Script != "" {
				t.Errorf("benchmark case %s must not declare a legacy script", item.ID)
			}
			if item.Benchmark.Baseline == "" || len(item.Benchmark.Datasets) == 0 || len(item.Benchmark.Variants) < 2 {
				t.Errorf("benchmark case %s must declare baseline, tier datasets and at least two variants", item.ID)
			}
		} else {
			if item.Script == "" || filepath.IsAbs(item.Script) || strings.HasPrefix(filepath.Clean(item.Script), "..") {
				t.Errorf("case %s script must stay under guide/: %s", item.ID, item.Script)
			} else if info, err := os.Stat(filepath.Join(root, item.Script)); err != nil || info.IsDir() {
				t.Errorf("case %s references missing script %s", item.ID, item.Script)
			}
		}
	}

	for _, report := range reports {
		if report.Schema != 1 {
			t.Errorf("report %s has unsupported schema %d", report.ID, report.Schema)
		}
		validationCase, ok := caseByID[report.Case]
		if !ok {
			t.Errorf("report %s references missing case %s", report.ID, report.Case)
			continue
		}
		referencedCases[report.Case] = true
		if !stableIDPattern.MatchString(report.ID) {
			t.Errorf("report has invalid ID %q", report.ID)
		}
		reportDate, err := time.Parse("2006-01-02", report.Date)
		if err != nil {
			t.Errorf("report %s has invalid date %q", report.ID, report.Date)
		} else if entry, exists := guides[report.Guide]; exists {
			updated, _ := time.Parse("2006-01-02", entry.Updated)
			if reportDate.After(updated) {
				t.Errorf("report %s date %s is later than guide updated %s", report.ID, report.Date, entry.Updated)
			}
		}
		if len(report.Tools) == 0 || len(report.Datasets) == 0 {
			t.Errorf("report %s must declare tools and dataset results", report.ID)
		}
		if report.Guide != validationCase.Guide {
			t.Errorf("report %s guide %s differs from case guide %s", report.ID, report.Guide, validationCase.Guide)
		}
		if _, err := os.Stat(filepath.Join(root, report.Summary)); err != nil {
			t.Errorf("report %s references missing summary %s", report.ID, report.Summary)
		}
		caseDatasets := stringSet(validationCase.Datasets...)
		for _, result := range report.Datasets {
			if _, ok := datasetByID[result.ID]; !ok {
				t.Errorf("report %s references missing dataset %s", report.ID, result.ID)
			}
			if _, ok := caseDatasets[result.ID]; !ok {
				t.Errorf("report %s dataset %s is not declared by case %s", report.ID, result.ID, report.Case)
			}
			if result.Repeats <= 0 || result.BaselineWallSeconds <= 0 || result.OptimizedWallSeconds <= 0 {
				t.Errorf("report %s has incomplete result for %s", report.ID, result.ID)
			}
			calculated := result.BaselineWallSeconds / result.OptimizedWallSeconds
			if math.Abs(calculated-result.Speedup) > 0.01 {
				t.Errorf("report %s speedup %.2f does not match calculated %.2f", report.ID, result.Speedup, calculated)
			}
		}
	}

	for _, dataset := range datasets {
		if dataset.Schema != 1 {
			t.Errorf("dataset %s has unsupported schema %d", dataset.ID, dataset.Schema)
		}
		if !stableIDPattern.MatchString(dataset.ID) {
			t.Errorf("dataset has invalid ID %q", dataset.ID)
		}
		if len(dataset.UsedBy.Guides) == 0 || len(dataset.UsedBy.Cases) == 0 {
			t.Errorf("dataset %s must declare used_by guides and cases", dataset.ID)
		}
		for _, id := range dataset.UsedBy.Guides {
			if _, ok := guides[id]; !ok {
				t.Errorf("dataset %s references missing guide %s", dataset.ID, id)
			}
		}
		for _, id := range dataset.UsedBy.Cases {
			validationCase, ok := caseByID[id]
			if !ok {
				t.Errorf("dataset %s references missing case %s", dataset.ID, id)
				continue
			}
			if !contains(validationCase.Datasets, dataset.ID) {
				t.Errorf("dataset %s references case %s without reverse dataset link", dataset.ID, id)
			}
			if !contains(dataset.UsedBy.Guides, validationCase.Guide) {
				t.Errorf("dataset %s case %s belongs to guide %s missing from used_by.guides", dataset.ID, id, validationCase.Guide)
			}
		}
	}

	for id := range caseByID {
		if !referencedCases[id] {
			t.Errorf("case %s is not referenced by guide evidence or a report", id)
		}
	}
	for id := range reportByID {
		if !referencedReports[id] {
			t.Errorf("report %s is not referenced by guide evidence", id)
		}
	}
}

func guideDevelopmentRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate catalog test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "guide"))
}

func loadCatalogFiles[T any](t *testing.T, root string) []T {
	t.Helper()
	var items []T
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var item T
		if err := yaml.Unmarshal(data, &item); err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}
		items = append(items, item)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func indexCatalog[T any](t *testing.T, items []T, id func(T) string) map[string]T {
	t.Helper()
	indexed := make(map[string]T, len(items))
	for _, item := range items {
		key := id(item)
		if key == "" {
			t.Error("catalog item has empty ID")
			continue
		}
		if _, exists := indexed[key]; exists {
			t.Errorf("duplicate catalog ID %s", key)
		}
		indexed[key] = item
	}
	return indexed
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
