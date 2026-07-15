---
schema: 2
id: bcftools.split-multiallelic
title: 拆分多等位位点
tool: bcftools
category: format
kind: practice
summary: 需要一行一个 ALT 时使用 norm -m -any，并检查 INFO/FORMAT 字段和基因型是否符合下游预期。
tags:
  - vcf
  - norm
  - multiallelic
  - biallelic
commands:
  - norm
level: intermediate
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 本条验证多等位拆分，不包含依赖参考 FASTA 的 left-align 验证。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-15"
  validations:
    - bcftools.split-multiallelic
  benchmarks: []
updated: "2026-07-15"
---

## 结论

下游工具要求双等位记录时，使用 `bcftools norm -m -any` 把每个 ALT 拆成独立记录。拆分会
重写与等位基因相关的 INFO/FORMAT 值和基因型，必须按下游语义检查结果。

## 适用场景

- 关联分析、注释或导入工具要求每条记录只有一个 ALT。
- 希望对每个 ALT 独立筛选或统计。
- 在比较不同 VCF 前统一多等位表示。

## 推荐方法

只拆分多等位位点：

```bash
bcftools norm -m -any -Oz -o "{output_vcf}" "{input_vcf}"
bcftools index "{output_vcf}"
```

如果还要 left-align 和校验 REF，必须使用与 VCF assembly 完全一致的参考序列：

```bash
bcftools norm -f "{reference_fasta}" -m -any -Oz \
  -o "{output_vcf}" "{input_vcf}"
```

## 为什么这样做

一个多等位记录可以包含多个 ALT，许多逐等位基因分析要求将其展开。`norm -m -any` 会按
ALT 拆行，并依据 VCF 的 Number 定义调整相关字段，而不只是简单复制原始文本行。

## 注意事项

- 拆分后记录数通常增加，不能继续期待与输入行数相同。
- `GT`、Number=A/R/G 的 INFO/FORMAT 字段需要重点检查。
- 拆分多等位位点不等同于 left-align；后者通常需要 `-f`。
- 参考 FASTA 必须与 VCF 使用同一 assembly 和 contig 命名。
- 不要在不了解后果时用 `-c s` 自动修正 REF 不匹配。

## 结果检查

确认输出不再包含多个 ALT，并对关键字段抽样：

```bash
bcftools query -f '%CHROM\t%POS\t%REF\t%ALT[\t%GT]\n' "{output_vcf}"
bcftools query -f '%ALT\n' "{output_vcf}" | grep ','
```

第二条命令没有输出，表示普通记录中没有逗号分隔的多个 ALT。

## 依据

- 官方行为：bcftools `norm -m -TYPE` 用于拆分多等位位点。
- 本地验证：bcftools 1.22.1 将官方 `test/check.vcf` 的 18 条记录拆成 24 条，输出 ALT
  均为单值，样本顺序保持不变。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.split-multiallelic`。
