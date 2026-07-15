---
schema: 2
id: bcftools.query-format
title: 正确编写 query 格式
tool: bcftools
category: format
kind: practice
summary: 位点字段写在方括号外，样本 FORMAT 字段写在方括号内，并显式输出制表符和换行。
tags:
  - query
  - format
  - samples
  - tsv
commands:
  - query
level: basic
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 本条验证常用的位点字段和逐样本 GT 展开格式。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-15"
  validations:
    - bcftools.query-format
  benchmarks: []
updated: "2026-07-15"
---

## 结论

使用 `bcftools query` 导出表格时，把 CHROM、POS、REF、ALT 等位点字段写在方括号外，把
`GT` 等逐样本 FORMAT 字段写在 `[...]` 内。格式中显式写出 `\t` 和 `\n`，便于稳定地产生 TSV。

## 适用场景

- 从 VCF/BCF 导出位点表格。
- 同时输出样本名和每个样本的基因型。
- 为 awk、R、Python 或数据库准备简单 TSV。

## 推荐方法

一行一个位点，并在后面展开所有样本：

```bash
bcftools query \
  -f '%CHROM\t%POS\t%REF\t%ALT[\t%SAMPLE=%GT]\n' \
  "{input_vcf}" > "{output_tsv}"
```

只列出样本名：

```bash
bcftools query --list-samples "{input_vcf}"
```

## 为什么这样做

方括号表示对选中的每个样本重复其中的格式。位点字段每条记录只输出一次，样本字段则按样本
顺序展开。直接用 `query` 比先输出完整 VCF 再按固定列号切割更清晰，也不容易受 FORMAT 顺序影响。

## 注意事项

- `%INFO/DP` 是位点 INFO，样本深度应放在方括号中按 FORMAT 字段读取。
- `%POS` 是 VCF 的 1-based 坐标；转换成 BED 时需要明确改为 0-based、half-open。
- 多个样本横向展开适合样本较少的表格；大队列可能产生非常宽的行。
- 缺失标签默认可能报错；不要在未检查字段定义时直接依赖 `-u` 输出点号。

## 结果检查

先查看少量行，并确认列数和样本顺序：

```bash
bcftools query -f '%CHROM\t%POS[\t%SAMPLE=%GT]\n' "{input_vcf}" | head
bcftools query --list-samples "{input_vcf}"
```

## 依据

- 官方行为：bcftools query 使用方括号循环展开样本字段。
- 本地验证：bcftools 1.22.1 对 18 条、2 个样本的官方测试 VCF 输出 18 行；`idSNP`
  正确展开为 `A=0/1` 和 `B=0/2`。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.query-format`。
