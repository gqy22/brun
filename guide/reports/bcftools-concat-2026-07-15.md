# bcftools concat medium pilot

本报告用于观察线程扩展趋势和 `--naive` 的数量级差异，不作为稳定 benchmark 结论。

## 输入与环境

| 项目 | 值 |
|---|---|
| 数据集 | `igsr-grch38-wgs-sites` |
| 压缩输入大小 | 977,746,786 bytes |
| 记录数 | 78,229,218 |
| contig 分片 | 23 |
| bcftools / htslib | 1.22.1 / 1.22.2 |
| CPU | Intel Xeon E5-2686 v4 @ 2.30 GHz |
| 逻辑 CPU | 72 |
| 内存 | 337,953,107,968 bytes |
| 文件系统 | ext4 |
| 日期 | 2026-07-15 |

输入按 contig 生成 Header 一致的 VCF.gz 分片。每种方案只运行一次，没有预热，操作系统缓存
状态未受控。concat 计时不包含后续索引和记录数检查。

## 命令

```bash
bcftools concat --no-version -Oz --threads 1 -f files.list
bcftools concat --no-version -Oz --threads 2 -f files.list
bcftools concat --no-version -Oz --threads 4 -f files.list
bcftools concat --no-version --naive -Oz -f files.list
```

## 原始结果

| 方案 | wall | user | system | 峰值 RSS | 输出大小 | 记录数 |
|---|---:|---:|---:|---:|---:|---:|
| 普通 1 线程 | 282.07 s | 216.69 s | 7.92 s | 11,260 KiB | 894,463,929 bytes | 78,229,218 |
| 普通 2 线程 | 185.70 s | 221.67 s | 7.01 s | 11,864 KiB | 894,463,929 bytes | 78,229,218 |
| 普通 4 线程 | 129.98 s | 226.65 s | 6.26 s | 13,000 KiB | 894,463,929 bytes | 78,229,218 |
| `--naive` | 8.22 s | 0.29 s | 1.73 s | 9,528 KiB | 894,186,962 bytes | 78,229,218 |

## 计算

```text
speedup(configuration) = wall(standard-t1) / wall(configuration)
wall reduction(A→B)    = (wall(A) - wall(B)) / wall(A) × 100%
average CPU cores      = (user + system) / wall
```

| 指标 | 结果 |
|---|---:|
| 1→2 线程 wall 降幅 | 34.165% |
| 2→4 线程 wall 降幅 | 30.005% |
| 1→4 线程 wall 降幅 | 53.919% |
| 2 线程相对 1 线程 | 1.519× |
| 4 线程相对 1 线程 | 2.170× |
| naive 相对 1 线程 | 34.315× |
| 1/2/4 线程平均 CPU 核心占用 | 0.796 / 1.231 / 1.792 |
| naive 平均 CPU 核心占用 | 0.246 |

## 当前判断

- 4 线程相对 2 线程仍减少约 30% wall time，本次数据不能说明 4 线程已到饱和点。
- 普通 concat 即使设置 4 线程，平均 CPU 使用约 1.79 核，说明线程只覆盖部分执行路径。
- Header 和文件类型兼容时，`--naive` 避免重新压缩，单轮快约 34.3 倍；这比继续增加压缩
  线程更值得优先考虑。
- naive 输出字节数与重新压缩输出不同，因此不能比较压缩文件哈希；本次仅验证两者记录数均为
  78,229,218。正式结论还需要增加解码记录 SHA-256。

## 原始数据位置

```text
.cache/guide-data/benchmarks/bcftools-concat/medium/20260715-144148-398941/
├── environment.tsv
├── commands.tsv
├── runs.tsv
└── summary.tsv
```

这些缓存文件不进入 Git；本报告保存可审阅摘要。完整实验仍需至少三轮、交替顺序，并补测
8/16 线程、`-Oz1`、`-Ob` 和任务级并行。
