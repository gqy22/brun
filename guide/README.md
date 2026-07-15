# 内置经验验证数据

此目录只保存可复现的定义和脚本。下载文件、索引、临时结果和完整基准报告统一写入
`.cache/guide-data/`，不进入 Git。

声明式性能实验的字段、采集语义和产物格式见 [`benchmark.md`](benchmark.md)。

```text
guide/
├── cases/             # 经验、数据集和验证脚本的稳定映射
├── cmd/data/          # 读取清单的下载器
├── cmd/bench/         # 声明式 benchmark runner
├── datasets/          # correctness/smoke/medium 分级清单
├── reports/           # 精选、可提交的基准摘要
└── scripts/bcftools/  # bcftools 正确性验证和数据准备

.cache/guide-data/
├── downloads/   # 原始下载
├── datasets/    # 解压、裁剪和建索引后的数据
├── work/        # 一次性验证输出
├── results/     # 正确性结果
└── benchmarks/  # 性能测试原始结果
```

下载数据：

```bash
make guide-data                 # 2.7 KB correctness，默认只下载这一层
make guide-data TIER=smoke      # 24 MB vcf.gz
make guide-data TIER=medium     # 978 MB vcf.gz，必须显式执行
```

运行 bcftools 正确性验证：

```bash
guide/scripts/bcftools/verify.sh
```

运行 `-Ou` 管道基准：

```bash
make guide-bench                                      # smoke：1 次预热、3 次正式重复
make guide-bench TIER=medium                          # medium：无预热、单轮试跑
make guide-bench TIER=medium REPEATS=3 WARMUPS=1      # medium 完整重复，耗时较长
```

性能实验由 `guide/cases/*.yaml` 中的 `benchmark` 声明驱动；统一 runner 负责数据集解析、预热、
平衡轮换、GNU time 采集、正确性检查和汇总。Make 入口使用本地 `bin/brun` 监控整场实验，
因此可在另一个终端使用 `brun list/show/logs/stop`；逐 variant 指标仍由 runner 采集。可以显式
选择案例：

```bash
make guide-bench CASE=bcftools.pipeline-benchmark TIER=smoke
```

smoke 默认先预热一次，再交替运行两种方案各三次；medium 默认不预热、各运行一次。
结果写入
`.cache/guide-data/benchmarks/`，包含环境、逐次 wall time/CPU/峰值内存、平均值和输出记录
SHA-256。原始 VCF、输出和基准结果均不进入 Git。

## 测试与计算信息放在哪里

- `cases/`：一条经验使用什么数据、脚本和断言。
- `scripts/`：实际命令、计时方式、正确性检查和计算公式。
- `.cache/guide-data/benchmarks/<topic>/<tier>/<run-id>/`：每次性能运行的环境、完整命令、
  逐轮原始值和机器计算摘要；这些大批量结果不进入 Git。
- `reports/`：从原始数据中选出的、可提交和审阅的结果摘要。

correctness 验证目前只保存可重复执行的断言，不长期保存每次临时输出；性能测试必须保留
`environment.tsv`、`state.tsv`、`commands.tsv`、`runs.tsv`、`checks.tsv`、`summary.tsv`、报告
草稿和对应 SHA-256 清单。`environment.tsv` 记录设备与文件系统，`state.tsv` 记录实验前后动态
负载。`runs.tsv` 同时保存 warmup 和 measured 原始轮次，只有 measured 进入汇总；
variant 按轮次循环移位，避免固定执行顺序。

## 证据链

每条内置经验通过 frontmatter 中的稳定 ID 引用验证案例和基准报告：

```text
internal/guide/content/*.md
  ├── evidence.validations ──> guide/cases/*.yaml
  └── evidence.benchmarks  ──> guide/reports/*.yaml

guide/cases/*.yaml
  ├── datasets ──> guide/datasets/*.yaml
  └── script   ──> guide/scripts/*
```

`go test ./internal/guide` 会检查这些引用、反向引用、脚本和报告摘要是否存在；
`make guide-verify` 会先执行结构检查，再运行 bcftools 正确性验证。

可通过 `BRUN_GUIDE_CACHE` 将下载和运行产物放到仓库外的缓存盘。新增公共数据集时必须固定
revision 和 SHA-256，并记录来源及许可证；VCF 文件本身不提交到 Git。只有公共数据无法覆盖、
而且需要进入自动回归测试的最小边界案例，才考虑增加人工 fixture。
