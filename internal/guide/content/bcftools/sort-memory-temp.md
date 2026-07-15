---
schema: 2
id: bcftools.sort-memory-temp
title: 控制 sort 内存与临时目录
tool: bcftools
category: performance
kind: practice
summary: 用 -m 控制每次内存缓冲，用 -T 把临时文件放到空间充足的本地高速磁盘。
tags:
  - sort
  - memory
  - temporary-files
  - io
commands:
  - sort
level: basic
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 本条只验证受限内存排序的正确性，不声明性能最优值。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-15"
  validations:
    - bcftools.sort-memory-temp
  benchmarks: []
updated: "2026-07-15"
---

## 结论

运行 `bcftools sort` 时，用 `-m` 明确单次内存缓冲上限，并用 `-T` 把临时文件放到空间充足、
吞吐较好的本地磁盘。内存越小，通常会产生越多临时文件，并不代表资源越省越快。

## 适用场景

- 输入 VCF/BCF 未按参考 contig 和坐标排序。
- 大文件排序需要限制峰值内存。
- 系统默认临时目录空间不足或位于较慢、共享的文件系统。

## 推荐方法

```bash
bcftools sort -m "{memory_per_buffer}" \
  -T "{local_tmp}/bcftools-sort.XXXXXX" \
  -Oz -o "{output_vcf}" "{input_vcf}"
bcftools index "{output_vcf}"
```

`XXXXXX` 会被替换为唯一字符串，适合多个任务同时运行。

## 为什么这样做

sort 会先在内存中积累记录，再写出临时分片并进行合并。`-m` 影响临时文件数量；`-T` 决定
这些文件写到哪里。实际速度往往同时受内存、临时盘吞吐和文件数量限制。

## 注意事项

- `-m` 是近似上限，不应当作进程总内存的严格硬限制。
- 设置过小可能产生大量临时文件，甚至触发 open files 上限。
- 并发多个 sort 时，要按任务总数计算内存和临时空间。
- 临时目录应在正常退出和失败后检查清理情况。
- 排序完成后仍要重新建立索引，旧索引不能继续使用。

## 结果检查

索引能成功建立是坐标顺序的基本检查，同时应比较排序前后的记录集合：

```bash
bcftools index "{output_vcf}"
bcftools index --nrecords "{output_vcf}"
```

## 依据

- 官方行为：`-m` 影响写入磁盘的临时文件数量，`-T` 指定唯一临时目录。
- 本地验证：bcftools 1.22.1 使用 `-m 1M` 和独立临时目录排序官方测试 VCF，输出可以建立
  索引，且排序前后的记录集合和数量一致。
- 本条没有性能报告，不声称固定内存值最快。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.sort-memory-temp`。
