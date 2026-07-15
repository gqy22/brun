---
schema: 2
id: bcftools.filter-sample-logic
title: 区分样本过滤中的 & 与 &&
tool: bcftools
category: quality
kind: pitfall
summary: FORMAT 条件要求同一样本同时满足时使用 &，允许不同样本分别满足时使用 &&。
tags:
  - filter
  - expression
  - samples
  - genotype
commands:
  - view
  - filter
  - query
level: intermediate
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 本条验证逐样本 FORMAT 条件的逻辑运算符差异。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-15"
  validations:
    - bcftools.filter-sample-logic
  benchmarks: []
updated: "2026-07-15"
---

## 结论

组合逐样本 FORMAT 条件时，`&` 要求同一个样本同时满足两侧条件；`&&` 在位点层面组合结果，
可以由不同样本分别满足两侧条件。`|` 与 `||` 也有对应的逐样本和位点级差异。

## 适用场景

- 根据多个 FORMAT 字段筛选基因型或位点。
- 查找同一样本同时满足深度和质量阈值的记录。
- 查找一个样本满足条件 A、另一个样本满足条件 B 的位点。

## 推荐方法

要求同一个样本既是杂合又满足深度阈值：

```bash
bcftools view -i 'GT="het" & FMT/DP>=20' "{input_vcf}"
```

只要求位点中存在杂合样本，同时存在纯合样本，两者可以不是同一人：

```bash
bcftools view -i 'GT="het" && GT="hom"' "{input_vcf}"
```

## 为什么这样做

FORMAT 表达式会对样本向量求值。单字符运算符保留样本对应关系，双字符运算符把两侧条件
提升到位点层面组合。两种写法都可能语法正确，但会回答不同的生物学问题。

## 注意事项

- 先用 `query` 输出样本名和相关 FORMAT 字段，在少量记录上核对表达式。
- `INFO/DP` 与 `FMT/DP` 含义不同，不要省略前缀后依赖猜测。
- `-i` 保留表达式为真的记录，`-e` 排除表达式为真的记录。
- 缺失值会影响比较结果，应显式考虑 `DP="."`、`GT="mis"` 等情况。
- 复杂表达式使用括号明确优先级。

## 结果检查

把命中的样本和基因型同时输出：

```bash
bcftools query -i 'GT="het" && GT="hom"' \
  -f '%CHROM\t%POS[\t%SAMPLE=%GT]\n' "{input_vcf}"
```

## 依据

- 官方行为：bcftools 手册区分 `&/|` 的逐样本逻辑和 `&&/||` 的位点级逻辑。
- 本地验证：bcftools 1.22.1 的官方测试 VCF 中，`GT="het" && GT="hom"` 命中 2 条记录，
  `GT="het" & GT="hom"` 命中 0 条，因为单个样本不可能同时为杂合和纯合。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.filter-sample-logic`。
