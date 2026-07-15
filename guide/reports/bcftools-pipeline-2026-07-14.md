# bcftools 管道中间格式基准

本报告比较两段等价的 bcftools 管道：

- `compressed-intermediate`：第一步输出 `-Oz`，第二步重新读取并输出 `-Oz`。
- `uncompressed-bcf`：第一步通过管道输出 `-Ou`，第二步输出 `-Oz`。

两种方案都筛选 `TYPE="snp"`，最终输出记录经解压后计算 SHA-256，结果一致。

## 环境

| 项目 | 值 |
|---|---|
| bcftools | 1.22.1 |
| htslib | 1.22.2 |
| 系统 | Linux 6.17.0-35-generic x86_64 |
| 最终压缩线程 | 1 |
| 日期 | 2026-07-14 |

## smoke

数据集：`igsr-grch38-chrx`

| 指标 | 值 |
|---|---:|
| 压缩输入大小 | 24,046,766 bytes |
| 记录数 | 106,963 |
| 样本数 | 2,548 |
| 预热 | 1 轮 |
| 正式重复 | 3 轮 |

| 方案 | 平均 wall time | 平均峰值 RSS |
|---|---:|---:|
| 压缩中间文件 | 61.847 s | 10,115 KiB |
| 未压缩 BCF 管道 | 21.693 s | 10,555 KiB |

该环境下 `-Ou` 管道 wall time 约为前者的 35.1%，即约 2.85 倍速度。这个结果只代表
当前工具版本、机器和数据组成，不能直接外推到其他流程。

## medium 试跑

数据集：`igsr-grch38-wgs-sites`

| 指标 | 值 |
|---|---:|
| 压缩输入大小 | 977,746,786 bytes |
| 记录数 | 78,229,218 |
| 样本数 | 0 |
| contig 数 | 23 |
| 预热 | 0 轮 |
| 正式重复 | 1 轮 |

| 方案 | wall time | 峰值 RSS |
|---|---:|---:|
| 压缩中间文件 | 617.00 s | 22,688 KiB |
| 未压缩 BCF 管道 | 299.14 s | 22,668 KiB |

该单轮试跑中 `-Ou` 管道约为 2.06 倍速度。由于没有预热和重复，该数据只用于确认
medium 量级、基准脚本稳定性及优化方向，不能视为稳定均值。

## 可复现入口

```bash
make guide-bench
make guide-bench TIER=medium
make guide-bench TIER=medium REPEATS=3 WARMUPS=1
```

原始数据的 URL、字节数、发布方 MD5 和本地 SHA-256 位于 `guide/datasets/`。
每次运行的逐轮原始数据保存在 `.cache/guide-data/benchmarks/`，不提交到 Git。
