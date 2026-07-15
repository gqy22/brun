---
schema: 2
id: bcftools.pipeline-uncompressed
title: 管道中间使用未压缩 BCF
tool: bcftools
category: performance
kind: practice
summary: 多个 bcftools 子命令串联时，中间使用 -Ou，最后一步再压缩输出。
tags:
  - vcf
  - bcf
  - pipeline
  - performance
commands:
  - view
  - filter
level: basic
status: benchmarked
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
    - bcftools.pipeline-equivalence
  benchmarks:
    - bcftools.pipeline-2026-07-14
updated: "2026-07-14"
---

## 结论

多个 bcftools 子命令通过管道连续处理时，中间输出优先使用 `-Ou`，只在最后一步使用
`-Oz` 或 `-Ob`。这样可以减少重复压缩、解压和格式转换。

## 适用场景

- 已验证的 `view` 和 `filter` 等流式子命令连续执行。
- 中间结果不需要长期保存或随机访问。
- 最终需要生成压缩 VCF 或 BCF。

如果需要保留中间文件用于审计、断点恢复或随机访问，应保存并为中间文件建立索引。
其他子命令也可能支持相同方式，但应先确认它们支持相应输入输出类型，不能从本条验证范围直接外推。

## 推荐方法

```bash
bcftools view -Ou "{input_vcf}" |
  bcftools filter -Oz -o "{output_vcf}"
```

## 为什么这样做

VCF 是文本格式，压缩 VCF 还涉及 BGZF 压缩。`-Ou` 输出未压缩 BCF，适合 bcftools
子命令之间传递，避免每一步把数据转换成文本或重新压缩。

## 并行与资源

该方法主要减少格式转换和压缩 I/O，不会把核心计算自动变成多线程。最终输出压缩时可以
根据 CPU 和磁盘吞吐设置少量压缩线程；同时运行多个任务时，应避免每个任务占用全部核心。

## 注意事项

- 管道必须启用 `pipefail`，否则前序命令失败可能被最后一个命令的退出状态掩盖。
- `-Ou` 数据不适合作为长期文件保存。
- 最终 VCF 若需要区域查询，应建立 `.tbi` 或 `.csi` 索引。
- 需要调试中间结果时，先保存一个明确命名的检查点文件。

## 结果检查

分别使用压缩中间文件和 `-Ou` 管道运行相同处理，比较两者的记录、样本顺序和关键 Header。
不要直接比较压缩文件字节，因为压缩参数和命令记录可能不同。

```bash
bcftools view -H "{baseline_vcf}" > baseline.records
bcftools view -H "{output_vcf}" > optimized.records
diff -u baseline.records optimized.records
```

## 依据

- 官方行为：bcftools 手册对通用输出类型和 `-Ou` 管道的说明。
- 本地验证：bcftools 1.22.1，使用 bcftools 1.22 官方 `test/check.vcf` 比较记录结果。
- smoke 实测：24,046,766 字节、106,963 条记录、2,548 个样本，三轮均值从
  61.847 秒降至 21.693 秒，约为 2.85 倍速度。
- medium 试跑：977,746,786 字节、78,229,218 条记录，单轮从 617.00 秒降至
  299.14 秒，约为 2.06 倍速度；单轮结果不作为稳定均值。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 证据 ID：`bcftools.pipeline-equivalence`、`bcftools.pipeline-2026-07-14`。
