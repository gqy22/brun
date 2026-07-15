---
schema: 2
id: bcftools.concat-vs-merge
title: 正确选择 concat 与 merge
tool: bcftools
category: workflow
kind: comparison
summary: 相同样本的坐标分片用 concat，不同样本集合的文件用 merge。
tags:
  - vcf
  - concat
  - merge
  - samples
commands:
  - concat
  - merge
level: basic
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 本条验证普通 VCF 分片和非重叠样本集合，不覆盖 gVCF 合并。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-15"
  validations:
    - bcftools.concat-vs-merge
  benchmarks:
    - bcftools.concat-2026-07-15
updated: "2026-07-15"
---

## 结论

沿坐标方向连接同一批样本的染色体或区间分片时使用 `bcftools concat`；沿样本方向合并
不同样本集合的文件时使用 `bcftools merge`。

## 适用场景

- `concat`：chr1、chr2 等分片具有完全相同、顺序一致的样本列。
- `merge`：不同文件包含不同样本，需要按位点组成一个多样本 VCF。
- 多等位记录的拆分和重组不属于这两种操作，应使用 `bcftools norm -m`。

## 推荐方法

连接按参考顺序排列的 contig 文件：

```bash
bcftools concat -Oz -o "{output_vcf}" "{ordered_vcf_files}"
```

合并不同样本文件：

```bash
bcftools merge -Oz -o "{output_vcf}" "{sample_vcf_files}"
```

## 为什么这样做

`concat` 主要把记录纵向接在一起，要求所有输入的样本列相同；`merge` 在相同或相邻位点上
协调等位基因，并把不同输入的样本列横向组合。两者解决的是不同维度的问题。

## 注意事项

- `concat` 输入必须按坐标排序，并按正确的 contig/区间顺序传入。
- 默认 `concat` 不接受坐标重叠的相邻文件；不要为了绕过错误盲目添加 `-a`。
- `merge` 通常要求输入经过 BGZF 压缩并建立索引。
- `merge` 可能调整等价变异的 REF/ALT 表示，不能只依赖压缩文件哈希判断结果。
- 样本名重复时先确认数据含义，不要直接使用 `--force-samples` 掩盖问题。

## 结果检查

至少检查样本顺序、记录数量、坐标顺序和基因型：

```bash
bcftools query --list-samples "{output_vcf}"
bcftools index --nrecords "{output_vcf}"
bcftools query -f '%CHROM\t%POS[\t%GT]\n' "{output_vcf}"
```

## 依据

- 官方行为：`concat` 面向相同样本集合，`merge` 面向不重叠的样本集合。
- 本地验证：bcftools 1.22.1 能 concat 四个 contig 分片并恢复原记录，也能把单样本 A/B
  文件 merge 为样本顺序 A/B 的 18 条记录；对 A/B 文件使用 concat 会因样本名不同而失败。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.concat-vs-merge`。
