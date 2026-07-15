---
schema: 2
id: bcftools.regions-vs-targets
title: 区分 regions 与 targets
tool: bcftools
category: workflow
kind: comparison
summary: 随机访问局部区域用 -r/-R，顺序扫描或排除区域用 -t/-T，并注意两者默认重叠语义不同。
tags:
  - vcf
  - regions
  - targets
  - index
commands:
  - view
level: intermediate
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 本条只验证 bcftools 1.22.1 的 view 行为。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-15"
  validations:
    - bcftools.regions-vs-targets
  benchmarks: []
updated: "2026-07-15"
---

## 结论

只读取少量局部区域时，对已索引输入优先使用 `-r/-R`；需要顺序扫描、排除区域或处理未索引
流时使用 `-t/-T`。不要把它们当作可以随意互换的参数：两者默认的重叠判断不同。

## 适用场景

- `-r/-R`：从大型、已索引的 VCF/BCF 中跳转到少量区域。
- `-t/-T`：顺序扫描输入，或使用 `^` 排除指定区域。
- 同时使用：先由 `-r` 缩小随机访问范围，再由 `-t` 在该范围内继续筛选。

## 推荐方法

随机访问已索引文件：

```bash
bcftools view -r "{region}" -Oz -o "{output_vcf}" "{input_vcf}"
```

顺序扫描并排除 contig：

```bash
bcftools view -t "^{excluded_contigs}" -Oz -o "{output_vcf}" "{input_vcf}"
```

需要统一为“POS 位于区间内”的语义时显式设置：

```bash
bcftools view -r "{region}" --regions-overlap 0 "{input_vcf}"
```

## 为什么这样做

`-r/-R` 使用索引跳转；`-t/-T` 顺序读取并丢弃目标之外的记录。默认情况下，regions 会考虑
记录与区间的重叠，targets 只检查 `POS`，所以跨越区间边界的 indel 可能只出现在前者结果中。

## 注意事项

- `-r/-R` 需要可用的 CSI 或 TBI 索引，`-t/-T` 不依赖随机访问。
- BED 文件是 0-based、half-open；普通三列表格默认是 1-based、闭区间。
- contig 名必须完全匹配，例如 `chr1` 与 `1` 不相同。
- `-R` 中相互重叠的区域可能产生重复或乱序记录。
- 需要稳定语义时显式设置 `--regions-overlap` 或 `--targets-overlap`。

## 结果检查

检查实际记录数，并重点查看跨越区间边界的 indel：

```bash
bcftools view -H -r "{region}" "{input_vcf}" | wc -l
bcftools view -H -t "{region}" "{input_vcf}" | wc -l
```

## 依据

- 官方行为：bcftools 手册说明 regions 使用索引跳转，targets 顺序扫描，且默认重叠模式不同。
- 本地验证：bcftools 1.22.1 中，一个从区间前方开始但与区间重叠的 deletion 被 `-r` 选中，
  默认 `-t` 和 `--regions-overlap 0` 均不选中。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.regions-vs-targets`。
