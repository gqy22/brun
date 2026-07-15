---
schema: 2
id: bcftools.threads-output-compression
title: 理解 --threads 的作用
tool: bcftools
category: parallel
kind: pitfall
summary: bcftools 通用 --threads 主要并行压缩输出，不会自动并行子命令的核心计算。
tags:
  - threads
  - compression
  - parallel
  - performance
commands:
  - view
level: basic
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 性能收益依赖输出格式、数据和存储，本条不声明固定加速比。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-15"
  validations:
    - bcftools.threads-output-compression
  benchmarks: []
updated: "2026-07-15"
---

## 结论

bcftools 1.22 的通用 `--threads` 用于 BCF/VCF.gz 输出流的压缩。增加这个参数不会自动把
`view`、`filter` 或其他子命令的核心算法变成多线程。

## 适用场景

- 最终输出使用 `-Ob` 或 `-Oz`，压缩可能成为瓶颈。
- 单个任务拥有多个可用 CPU，并且存储吞吐能够跟上。
- 需要控制多个并发任务各自占用的压缩线程数。

## 推荐方法

只在压缩输出时分配适量线程：

```bash
bcftools view -Oz --threads "{compression_threads}" \
  -o "{output_vcf}" "{input_vcf}"
```

如果任务可以按 contig 独立处理，优先评估任务级并行；不要只把 `--threads` 调到全部核心。

## 为什么这样做

通用线程参数的作用范围是输出压缩，而不是每个子命令的全部处理阶段。因此总耗时可能受
筛选表达式、解析、输入解压、磁盘或网络限制，线程增加后也不一定继续加速。

## 注意事项

- 输出 `-Ov` 或 `-Ou` 时，不要期待通用 `--threads` 带来压缩加速。
- 某些子命令或插件可能有自己的并行能力，应分别查看对应帮助。
- 多任务并发时，总线程数应按所有任务求和，避免 CPU 过度订阅。
- 线程收益必须在代表性数据上测量，不能直接复用别人的线程数。

## 结果检查

线程设置不应改变解码后的记录：

```bash
diff -u \
  <(bcftools view -H "{baseline_vcf}") \
  <(bcftools view -H "{threaded_vcf}")
```

## 依据

- 官方行为：bcftools 1.22 手册说明通用 `--threads` 当前只用于 `-Ob/-Oz` 输出流压缩。
- 本地验证：bcftools 1.22.1 使用两个压缩线程生成的 VCF.gz 与输入记录一致。
- 本条没有性能报告，不声称固定加速倍数。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.threads-output-compression`。
