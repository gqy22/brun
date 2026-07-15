---
schema: 2
id: bcftools.parallel-by-contig
title: 按染色体并行处理 VCF
tool: bcftools
category: parallel
kind: practice
summary: 对区域之间相互独立的操作，优先按完整染色体拆分，并按参考顺序合并。
tags:
  - vcf
  - parallel
  - contig
  - performance
commands:
  - view
  - filter
  - annotate
  - concat
level: intermediate
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 更早版本可能支持相同选项，但本条没有逐版本验证。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-14"
  validations:
    - bcftools.parallel-by-contig
  benchmarks: []
updated: "2026-07-14"
---

## 结论

对于能够按区域独立处理的 VCF 操作，按完整染色体拆分通常是扩大计算并行度且容易验证
正确性的首选方案。完成后必须按照参考基因组的 contig 顺序合并。

## 适用场景

- 已建立索引的 VCF 或 BCF。
- `view`、逐记录 `filter` 和不依赖全局状态的 `annotate`。
- 每条染色体都可以独立生成结果的流程。

依赖全局统计、跨区域上下文或复杂窗口状态的操作，需要先确认算法语义，不能直接套用。

## 推荐方法

每条染色体独立运行：

```bash
bcftools view -r "{contig}" -Oz \
  -o "{output_dir}/{contig}.vcf.gz" "{input_vcf}"
```

按参考基因组顺序合并：

```bash
bcftools concat -Oz -o "{output_vcf}" "{ordered_contig_files}"
bcftools index "{output_vcf}"
```

## 为什么这样做

完整染色体之间没有人为切出的区间边界，任务容易独立失败和重试，也容易按参考序列顺序
恢复完整结果。相比任意固定窗口，它需要处理的边界风险更少。

## 并行与资源

并行单位是染色体任务，而不是单个 bcftools 进程内部的计算线程。并发任务数应同时受可用
内存、CPU 和存储吞吐限制。任务数较多时，应降低每个任务用于输出压缩的线程数。

## 注意事项

- 输入必须具有可用索引。
- 不要按文件名字典序猜测染色体顺序，例如 `chr10` 会排在 `chr2` 前面。
- 所有分片的 Header 和样本顺序必须一致。
- `norm` 等可能受参考上下文影响的操作优先按完整染色体拆分。
- 固定窗口拆分需要单独设计重叠区间、裁边和去重规则。

## 结果检查

将单任务完整处理结果与“按染色体处理后 concat”的结果转换为相同表示，比较记录内容、
记录数量、样本顺序和 contig 顺序。

```bash
bcftools view -H "{baseline_vcf}" > baseline.records
bcftools view -H "{output_vcf}" > parallel.records
diff -u baseline.records parallel.records
```

## 依据

- 官方行为：bcftools 支持对已索引输入使用区域选择，并使用 `concat` 连接坐标不重叠的文件。
- 实践建议：以完整 contig 作为默认拆分边界，固定窗口拆分属于需要额外验证的高级方案。
- 本地验证：bcftools 1.22.1，使用官方 `test/check.vcf` 的四个 contig 验证分片合并结果。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.parallel-by-contig`。
