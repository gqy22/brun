package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type benchmarkCase struct {
	Schema     int               `yaml:"schema"`
	ID         string            `yaml:"id"`
	Guide      string            `yaml:"guide"`
	Datasets   []string          `yaml:"datasets"`
	Requires   benchmarkRequires `yaml:"requires"`
	Assertions []string          `yaml:"assertions"`
	Benchmark  benchmarkSpec     `yaml:"benchmark"`
}

type benchmarkRequires struct {
	Tools []string `yaml:"tools"`
}

type benchmarkSpec struct {
	Baseline        string             `yaml:"baseline"`
	Datasets        map[string]string  `yaml:"datasets"`
	Setup           *benchmarkSetup    `yaml:"setup"`
	Warmups         int                `yaml:"warmups"`
	Repeats         int                `yaml:"repeats"`
	Order           string             `yaml:"order"`
	CachePolicy     string             `yaml:"cache_policy"`
	OutputExtension string             `yaml:"output_extension"`
	Variables       map[string]string  `yaml:"variables"`
	Versions        []versionCommand   `yaml:"versions"`
	Variants        []benchmarkVariant `yaml:"variants"`
	Checks          []benchmarkCheck   `yaml:"checks"`
}

type benchmarkSetup struct {
	Command []string `yaml:"command"`
}

type versionCommand struct {
	Name    string   `yaml:"name"`
	Command []string `yaml:"command"`
}

type benchmarkVariant struct {
	ID      string              `yaml:"id"`
	Matrix  map[string][]string `yaml:"matrix"`
	Command []string            `yaml:"command"`
	Values  map[string]string   `yaml:"-"`
}

type benchmarkCheck struct {
	ID      string   `yaml:"id"`
	Type    string   `yaml:"type"`
	Command []string `yaml:"command"`
}

func loadBenchmarkCase(path string) (benchmarkCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return benchmarkCase{}, err
	}
	var item benchmarkCase
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&item); err != nil {
		return benchmarkCase{}, fmt.Errorf("解析 %s: %w", path, err)
	}
	if err := validateBenchmarkCase(item); err != nil {
		return benchmarkCase{}, fmt.Errorf("%s: %w", path, err)
	}
	return item, nil
}

func validateBenchmarkCase(item benchmarkCase) error {
	if item.Schema != 1 {
		return fmt.Errorf("schema 必须为 1，当前为 %d", item.Schema)
	}
	if strings.TrimSpace(item.ID) == "" {
		return errors.New("id 不能为空")
	}
	spec := item.Benchmark
	if spec.Baseline == "" || len(spec.Datasets) == 0 || len(spec.Variants) < 2 {
		return errors.New("benchmark 必须声明 baseline、datasets 和至少两个 variants")
	}
	if spec.Warmups < 0 || spec.Repeats <= 0 {
		return errors.New("warmups 必须非负且 repeats 必须为正整数")
	}
	if spec.Order != "balanced" {
		return fmt.Errorf("当前只支持 order=balanced，收到 %q", spec.Order)
	}
	if spec.CachePolicy == "" {
		return errors.New("benchmark.cache_policy 不能为空")
	}
	if spec.CachePolicy != "uncontrolled" && spec.CachePolicy != "warm" && spec.CachePolicy != "cold" {
		return fmt.Errorf("未知 cache_policy %q", spec.CachePolicy)
	}
	seen := make(map[string]bool, len(spec.Variants))
	for _, variant := range spec.Variants {
		if variant.ID == "" || len(variant.Command) == 0 {
			return errors.New("每个 variant 必须声明 id 和 command")
		}
		if seen[variant.ID] {
			return fmt.Errorf("variant ID 重复: %s", variant.ID)
		}
		seen[variant.ID] = true
	}
	expanded, err := expandVariants(spec.Variants, nil)
	if err != nil {
		return err
	}
	seen = make(map[string]bool, len(expanded))
	for _, variant := range expanded {
		seen[variant.ID] = true
	}
	if !seen[spec.Baseline] {
		return fmt.Errorf("baseline %q 不在 variants 中", spec.Baseline)
	}
	if spec.Setup != nil && len(spec.Setup.Command) == 0 {
		return errors.New("setup.command 不能为空")
	}
	for _, check := range spec.Checks {
		if check.ID == "" || check.Type != "stdout-sha256-equal" || len(check.Command) == 0 {
			return fmt.Errorf("check 必须声明 id、command，且当前 type 只支持 stdout-sha256-equal")
		}
	}
	return nil
}

func expandVariants(variants []benchmarkVariant, overrides map[string][]string) ([]benchmarkVariant, error) {
	knownMatrices := make(map[string]bool)
	var expanded []benchmarkVariant
	seenIDs := make(map[string]bool)
	for _, variant := range variants {
		if len(variant.Matrix) == 0 {
			if seenIDs[variant.ID] {
				return nil, fmt.Errorf("展开后的 variant ID 重复: %s", variant.ID)
			}
			seenIDs[variant.ID] = true
			expanded = append(expanded, variant)
			continue
		}
		keys := make([]string, 0, len(variant.Matrix))
		for key, values := range variant.Matrix {
			knownMatrices[key] = true
			if override, ok := overrides[key]; ok {
				values = override
			}
			if key == "" || len(values) == 0 {
				return nil, fmt.Errorf("variant %s 的 matrix %q 没有值", variant.ID, key)
			}
			variant.Matrix[key] = values
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var visit func(int, map[string]string) error
		visit = func(index int, values map[string]string) error {
			if index == len(keys) {
				id := variant.ID
				for _, key := range keys {
					id += "-" + slugID(key) + "-" + slugID(values[key])
				}
				if seenIDs[id] {
					return fmt.Errorf("展开后的 variant ID 重复: %s", id)
				}
				seenIDs[id] = true
				expanded = append(expanded, benchmarkVariant{ID: id, Command: variant.Command, Values: cloneStrings(values)})
				return nil
			}
			key := keys[index]
			for _, value := range variant.Matrix[key] {
				if value == "" {
					return fmt.Errorf("variant %s 的 matrix %s 包含空值", variant.ID, key)
				}
				values[key] = value
				if err := visit(index+1, values); err != nil {
					return err
				}
			}
			delete(values, key)
			return nil
		}
		if err := visit(0, make(map[string]string, len(keys))); err != nil {
			return nil, err
		}
	}
	for key, values := range overrides {
		if !knownMatrices[key] {
			return nil, fmt.Errorf("未知 matrix 覆盖 %q", key)
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("matrix 覆盖 %q 没有值", key)
		}
	}
	return expanded, nil
}

func slugID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, character := range value {
		valid := character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
		if valid {
			builder.WriteRune(character)
			lastDash = false
		} else if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "value"
	}
	return result
}

type plannedRun struct {
	Phase   string
	Repeat  int
	Order   int
	Variant benchmarkVariant
}

func buildPlan(variants []benchmarkVariant, warmups, repeats int) []plannedRun {
	plan := make([]plannedRun, 0, (warmups+repeats)*len(variants))
	order := 0
	appendRounds := func(phase string, rounds int) {
		for round := 0; round < rounds; round++ {
			for offset := range len(variants) {
				index := (round + offset) % len(variants)
				order++
				plan = append(plan, plannedRun{
					Phase:   phase,
					Repeat:  round + 1,
					Order:   order,
					Variant: variants[index],
				})
			}
		}
	}
	appendRounds("warmup", warmups)
	appendRounds("measured", repeats)
	return plan
}

var placeholderPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func expandArguments(arguments []string, values map[string]string) ([]string, error) {
	expanded := make([]string, len(arguments))
	for i, argument := range arguments {
		var missing string
		expanded[i] = placeholderPattern.ReplaceAllStringFunc(argument, func(token string) string {
			key := token[1 : len(token)-1]
			value, ok := values[key]
			if !ok {
				missing = key
				return token
			}
			return value
		})
		if missing != "" {
			return nil, fmt.Errorf("未声明占位符 {%s}", missing)
		}
	}
	return expanded, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
