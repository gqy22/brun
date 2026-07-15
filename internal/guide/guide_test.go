package guide

import (
	"strings"
	"testing"
)

func TestLoadEmbeddedEntries(t *testing.T) {
	entries, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("Load() returned %d entries, want at least 2", len(entries))
	}
	for _, entry := range entries {
		if entry.SourcePath == "" {
			t.Fatalf("entry %q has no source path", entry.ID)
		}
	}
}

func TestSearchMatchesMetadataAndBody(t *testing.T) {
	entries, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	matches := Search(entries, "bcftools contig")
	found := false
	for _, match := range matches {
		if match.ID == "bcftools.parallel-by-contig" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Search() = %+v, want results to contain parallel-by-contig", matches)
	}
	if matches := Search(entries, "不存在的关键词"); len(matches) != 0 {
		t.Fatalf("Search() returned unexpected matches: %+v", matches)
	}
}

func TestParseRejectsUnknownMetadata(t *testing.T) {
	data := strings.Replace(
		validDocument(),
		"updated: \"2026-07-14\"",
		"updated: \"2026-07-14\"\nunknown_field: true",
		1,
	)
	if _, err := Parse([]byte(data)); err == nil || !strings.Contains(err.Error(), "unknown_field") {
		t.Fatalf("Parse() error = %v, want unknown_field error", err)
	}
}

func TestParseRejectsMissingSection(t *testing.T) {
	data := strings.Replace(validDocument(), "## 推荐方法\n", "", 1)
	if _, err := Parse([]byte(data)); err == nil || !strings.Contains(err.Error(), "推荐方法") {
		t.Fatalf("Parse() error = %v, want missing section error", err)
	}
}

func TestParseRejectsStatusWithoutRequiredEvidence(t *testing.T) {
	data := strings.Replace(validDocument(), "validations: [bcftools.example]", "validations: []", 1)
	if _, err := Parse([]byte(data)); err == nil || !strings.Contains(err.Error(), "verified") {
		t.Fatalf("Parse() error = %v, want verified evidence error", err)
	}
}

func validDocument() string {
	return `---
schema: 2
id: bcftools.example
title: 示例
tool: bcftools
category: workflow
kind: practice
summary: 示例摘要
tags: [vcf]
commands: [view]
level: basic
status: verified
versions:
  tested: ["1.22.1"]
  documented: ["1.22"]
evidence:
  docs:
    - title: Manual
      url: https://example.com/manual
      checked: "2026-07-14"
  validations: [bcftools.example]
  benchmarks: []
updated: "2026-07-14"
---
## 结论
x
## 适用场景
x
## 推荐方法
x
## 注意事项
x
## 依据
x
`
}
