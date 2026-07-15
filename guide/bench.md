# 待验证性能经验

此文件只记录需要代表性算力和数据规模的实验，不把尚未运行的假设写成内置经验。

## bcftools concat 线程饱和点

状态：medium 单轮 pilot 已完成；完整多轮和更高线程矩阵暂缓。

轻量 pilot 入口（默认 1/2/4 线程和 naive，各运行一次）：

```bash
make guide-bench-concat
```

原始结果保存到 `.cache/guide-data/benchmarks/bcftools.concat-benchmark/medium/<run-id>/`。

2026-07-15 pilot 使用 23 个 contig、78,229,218 条记录，结果为：普通 concat 1/2/4 线程
分别耗时 282.07/185.70/129.98 秒，`--naive` 耗时 8.22 秒。详情见
`reports/bcftools-concat-2026-07-15.md`。单轮且缓存未受控，不能据此断言 4 线程是饱和点。

目标：确定普通 `concat` 在压缩输出时的有效线程数，并比较 `--naive`、输出格式和压缩等级。

计划使用现有 978 MB medium VCF，按 contig 生成 Header 一致的分片。测试矩阵：

```text
普通 -Oz：  threads 1/2/4/8/16
普通 -Oz1： threads 1/2/4/8/16
普通 -Ob：  threads 1/2/4/8/16
-Ou：       threads 1/8，验证通用线程对未压缩输出基本无效
--naive：   与普通 concat 比较，不做完整线程矩阵
```

每组预热一次、正式运行三次，交替执行。记录 wall/user/system time、峰值 RSS、输出大小和
记录 SHA-256。线程翻倍后 wall time 改善不足 5% 时视为进入收益平台；推荐值取接近最佳结果
的最少线程，而不是占用核心最多的配置。

额外比较固定总核心数下的任务级并行，例如 `1×16`、`2×8`、`4×4`、`8×2`。最终结论必须
注明 CPU、存储、输入规模、分片数、输出格式和 bcftools/htslib 版本。

注意：`--naive` 不重新压缩，要求输入类型和 Header 一致；只能使用带兼容性检查的
`--naive`，不能把 `--naive-force` 作为常规优化。
