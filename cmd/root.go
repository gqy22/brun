package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/biotools/brun/internal"
)

// GenerateScriptTemplate 生成脚本模板
func GenerateScriptTemplate(project, name, condaInfo, created string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# brun: %s | created: %s
# ============================================================
#
# %s.sh - <一句话描述这个脚本做什么>
#
# 流程:
#   1. 步骤一
#   2. 步骤二
#
# 依赖:
#   conda: %s
#
# 输入:
#   data/input.fastq.gz
#
# 输出:
#   results/output.bam
#
# 用法: brun run -- bash %s.sh
# ============================================================

WORKDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd"

# ---- 配置（按需修改）----
REF="${WORKDIR}/ref/genome.fa"
THREADS=16

# ---- 主逻辑 ----
echo "[$(date '+%%Y-%%m-%%d %%H:%%M:%%S')] 开始"
bwa mem -t "$THREADS" "$REF" data/input.fastq.gz > results/output.sam
echo "[$(date '+%%Y-%%m-%%d %%H:%%M:%%S')] 完成"
`, project, created, name, condaInfo, name)
}

// NextScriptName 计算下一个脚本文件名（同名递增后缀，不同名新编号）
func NextScriptName(dir, name string) (string, int) {
	entries, _ := os.ReadDir(dir)

	// 找所有 NN_name*.sh 的文件，提取编号和基础名
	type entry struct {
		num    int
		base   string // 去掉后缀数字的 base，如 align2 → align
		suffix int    // 后缀数字，如 align2 → 2
	}
	var existing []entry

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".sh")
		// 解析 "NN_basename" 或 "NN_basenameN"
		parts := strings.SplitN(base, "_", 2)
		if len(parts) < 2 {
			continue
		}
		var num int
		if _, err := fmt.Sscanf(parts[0], "%d", &num); err != nil {
			continue
		}
		basename := parts[1]
		// 检查 basename 尾部是否有数字后缀（如 align2 → base=align, suffix=2）
		suffix := 0
		j := len(basename) - 1
		for j >= 0 && basename[j] >= '0' && basename[j] <= '9' {
			j--
		}
		if j < len(basename)-1 {
			fmt.Sscanf(basename[j+1:], "%d", &suffix)
			basename = basename[:j+1]
		}
		existing = append(existing, entry{num, basename, suffix})
	}

	if len(existing) == 0 {
		return fmt.Sprintf("01_%s.sh", name), 1
	}

	// 找同 base 的最大编号和最大后缀
	maxNum := 0
	maxSuffix := 0
	hasSameBase := false
	for _, e := range existing {
		if e.num > maxNum {
			maxNum = e.num
		}
		if e.base == name {
			hasSameBase = true
			if e.suffix > maxSuffix {
				maxSuffix = e.suffix
			}
		}
	}

	if hasSameBase {
		nextSuffix := maxSuffix + 1
		if nextSuffix < 2 {
			nextSuffix = 2
		}
		nextNum := maxNum
		for _, e := range existing {
			if e.base == name && e.num == nextNum {
				nextNum = e.num
				break
			}
		}
		return fmt.Sprintf("%02d_%s%d.sh", nextNum, name, nextSuffix), maxNum + 1
	}

	// 不同名：新编号
	return fmt.Sprintf("%02d_%s.sh", maxNum+1, name), maxNum + 1
}

// DetectCondaEnv 检测当前 conda 环境
func DetectCondaEnv() (envStr string) {
	env := os.Getenv("CONDA_DEFAULT_ENV")
	if env == "" {
		return ""
	}
	prefix := os.Getenv("CONDA_PREFIX")

	// 尝试获取 Python 版本
	pythonVer := ""
	if out, err := exec.Command("python3", "--version").Output(); err == nil {
		pythonVer = strings.TrimSpace(string(out))
	}

	// 尝试获取关键包版本（快速，<200ms）
	packages := ""
	if prefix != "" {
		listPath := filepath.Join(prefix, "conda-meta/history")
		if data, err := os.ReadFile(listPath); err == nil {
			// 从 history 提取最近安装的几个关键包
			lines := strings.Split(string(data), "\n")
			count := 0
			seen := make(map[string]bool)
			keyTools := []string{"samtools", "bcftools", "bedtools", "bwa", "plink", "gatk"}
			for i := len(lines) - 1; i >= 0 && count < 5; i-- {
				for _, tool := range keyTools {
					if strings.Contains(lines[i], tool) && !seen[tool] {
						if packages != "" {
							packages += ", "
						}
						packages += tool
						seen[tool] = true
						count++
					}
				}
			}
		}
	}

	result := env
	if pythonVer != "" {
		result += fmt.Sprintf(" (%s)", pythonVer)
	}
	if packages != "" {
		result += fmt.Sprintf(" [%s]", packages)
	}
	return result
}
	type CleanItem struct {
	RunID  string
	Age    string
	Size   string
	Reason string
}

func GenerateInitYAML(project string) string {
	return fmt.Sprintf(`project: %s

capture:
  scripts:
    - "scripts/**/*.py"
    - "scripts/**/*.R"
    - "*.sh"
    - "Snakefile"
    - "*.nf"
  configs:
    - "configs/**/*.yaml"
    - "configs/**/*.yml"
    - "samples.tsv"
  outputs:
    - "results/**/*"
    - "reports/**/*"

ignore:
  - ".git/**"
  - ".snakemake/**"
  - ".nextflow/**"
  - "work/**"
  - "tmp/**"
  - "__pycache__/**"
  - "*.tmp"
  - "*.swp"
`, project)
}

func WriteInitFile(dir, project string) error {
	yaml := GenerateInitYAML(project)
	return os.WriteFile(filepath.Join(dir, "brun.yaml"), []byte(yaml), 0644)
}

func FormatCleanSummary(items []CleanItem, dryRun bool) string {
	var b strings.Builder
	if len(items) == 0 {
		return "无需清理。\n"
	}
	prefix := "Will remove"
	if dryRun {
		prefix = "Would remove"
	}
	fmt.Fprintf(&b, "%s %d runs:\n", prefix, len(items))
	for _, item := range items {
		fmt.Fprintf(&b, "  %s  (%s, %s) - %s\n", item.RunID, item.Age, item.Size, item.Reason)
	}
	return b.String()
}

func BuildRerunCommand(run *internal.Run, newCWD string, inheritTags bool) (string, string) {
	cwd := run.CWD
	if newCWD != "" {
		cwd = newCWD
	}
	return run.Command, cwd
}

func BuildMetadataYAML(run *internal.Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "id: %s\n", run.ID)
	if run.Name != "" {
		fmt.Fprintf(&b, "name: %s\n", run.Name)
	}
	fmt.Fprintf(&b, "project: %s\n", run.Project)
	fmt.Fprintf(&b, "command: %s\n", run.Command)
	fmt.Fprintf(&b, "status: %s\n", run.Status)
	fmt.Fprintf(&b, "exit_code: %d\n", run.ExitCode)
	fmt.Fprintf(&b, "cwd: %s\n", run.CWD)
	if run.StartedAt != "" {
		fmt.Fprintf(&b, "started_at: %s\n", run.StartedAt)
	}
	if run.EndedAt != "" {
		fmt.Fprintf(&b, "ended_at: %s\n", run.EndedAt)
	}
	if run.DurationMs > 0 {
		fmt.Fprintf(&b, "duration_ms: %d\n", run.DurationMs)
	}
	if run.GitCommit != "" {
		fmt.Fprintf(&b, "git_commit: %s\n", run.GitCommit)
	}
	if run.GitDirty {
		b.WriteString("git_dirty: true\n")
	}
	return b.String()
}
