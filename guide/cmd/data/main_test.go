package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDatasetsAndSelectTier(t *testing.T) {
	root := t.TempDir()
	document := `
schema: 1
id: example-correctness
tier: correctness
description: example
source:
  format: vcf.gz
  url: https://example.test/input.vcf.gz
  filename: input.vcf.gz
  bytes: 3
  checksums:
    - algorithm: sha256
      value: abc
metadata:
  assembly: test
  records: 1
  samples: 0
  contigs: 1
license: test
accessed: "2026-07-14"
used_by:
  guides: []
  cases: []
`
	if err := os.WriteFile(filepath.Join(root, "example.yaml"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	datasets, err := loadDatasets(root)
	if err != nil {
		t.Fatal(err)
	}
	selected := selectDatasets(datasets, "correctness", "")
	if len(selected) != 1 || selected[0].ID != "example-correctness" {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestDownloadArtifactAdoptsCompletedPartialFile(t *testing.T) {
	dir := t.TempDir()
	data := []byte("complete download")
	sum := sha256.Sum256(data)
	item := artifact{
		URL:      "https://invalid.example.test/input.vcf.gz",
		Filename: "input.vcf.gz",
		Bytes:    int64(len(data)),
		Checksums: []checksum{{
			Algorithm: "sha256",
			Value:     hex.EncodeToString(sum[:]),
		}},
	}
	partial := filepath.Join(dir, item.Filename+".part")
	if err := os.WriteFile(partial, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := downloadArtifact(context.Background(), &http.Client{}, dir, item); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, item.Filename)); err != nil {
		t.Fatalf("completed file not adopted: %v", err)
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatalf("partial file still exists: %v", err)
	}
}

func TestVerifyArtifactChecksSizeAndDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.vcf.gz")
	data := []byte("vcf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	item := artifact{
		Filename: path,
		Bytes:    int64(len(data)),
		Checksums: []checksum{{
			Algorithm: "sha256",
			Value:     hex.EncodeToString(sum[:]),
		}},
	}
	if ok, reason := verifyArtifact(path, item); !ok {
		t.Fatalf("verifyArtifact() failed: %s", reason)
	}
	item.Bytes++
	if ok, _ := verifyArtifact(path, item); ok {
		t.Fatal("verifyArtifact() accepted wrong size")
	}
}
