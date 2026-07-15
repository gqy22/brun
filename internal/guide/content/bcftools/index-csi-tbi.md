---
schema: 2
id: bcftools.index-csi-tbi
title: 默认使用 CSI 索引
tool: bcftools
category: format
kind: practice
summary: 新流程默认使用 CSI；只有下游明确要求 TBI 时才生成 TBI，并注意其 contig 长度限制。
tags:
  - index
  - csi
  - tbi
  - random-access
commands:
  - index
level: basic
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 本条验证 VCF.gz 的 CSI/TBI 建立和区域查询，不覆盖超长 contig 实测。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-15"
  validations:
    - bcftools.index-csi-tbi
  benchmarks: []
updated: "2026-07-15"
---

## 结论

新流程优先使用 bcftools 默认生成的 CSI 索引。只有下游软件明确要求 `.tbi` 时才使用 TBI，
因为 TBI 支持的 contig 最大长度小于 CSI。

## 适用场景

- 为坐标排序、BGZF 压缩的 VCF.gz 或 BCF 建立随机访问索引。
- 使用 `-r/-R` 按区域查询。
- 在 CSI 的范围能力与旧工具的 TBI 兼容性之间选择。

## 推荐方法

默认 CSI：

```bash
bcftools index "{input_vcf_gz}"
```

下游明确要求 TBI 时：

```bash
bcftools index --tbi "{input_vcf_gz}"
```

## 为什么这样做

bcftools 默认创建 CSI。官方文档给出的最大 contig 长度为 CSI `2^31`、TBI `2^29`。CSI
更适合作为新流程默认值，而 TBI 的主要价值是兼容只识别 `.tbi` 的既有工具。

## 注意事项

- 普通 gzip 不等于 BGZF，VCF.gz 必须使用兼容 BGZF 的方式压缩。
- 建索引前必须按 contig 和坐标排序。
- 同时存在 `.csi` 和 `.tbi` 时，bcftools 会优先尝试 CSI；避免遗留过期索引。
- BCF 使用 CSI，不要为 BCF 设计 TBI 路径。
- 使用 `-f` 覆盖索引前，确认数据文件没有被意外替换。

## 结果检查

建立索引后执行区域读取并检查记录数：

```bash
bcftools index --nrecords "{input_vcf_gz}"
bcftools view -H -r "{region}" "{input_vcf_gz}"
```

## 依据

- 官方行为：bcftools 默认创建 CSI；CSI 支持到 `2^31`，TBI 支持到 `2^29` 的 contig 长度。
- 本地验证：bcftools 1.22.1 分别为相同 VCF.gz 创建 `.csi` 和 `.tbi`，区域查询结果一致。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.index-csi-tbi`。
