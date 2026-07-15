# Benchmark 规范

brun guide benchmark 使用声明式案例统一运行性能实验。案例负责描述数据集、命令和正确性检查，
runner 负责执行顺序、资源采集、统计和产物格式；它不是 DAG 或工作流调度器。

## 运行

```bash
make guide-bench CASE=bcftools.pipeline-benchmark TIER=smoke
make guide-bench CASE=bcftools.pipeline-benchmark TIER=medium REPEATS=3 WARMUPS=1
```

Make 入口会先构建本地 `bin/brun`，再用前台 `brun run` 包住 benchmark runner。这样长实验可通过
`brun list/show/logs/stop` 监控和控制；runner 内部仍以 GNU `/usr/bin/time` 采集每个 variant，
Brun 的进程采样不作为 variant 间性能比较的数据源。

也可以直接调用：

```bash
go run ./guide/cmd/bench --tier smoke bcftools.pipeline-benchmark
```

runner 从 `guide/cases/` 按稳定 ID 查找案例，从 `guide/datasets/` 解析该 tier 的数据集。
下载文件必须已经位于 `${BRUN_GUIDE_CACHE:-.cache/guide-data}/downloads/<dataset-id>/`。

## 案例结构

```yaml
schema: 1
id: example.benchmark
guide: example.guide
datasets: [example-smoke]
requires:
  tools: [/usr/bin/time, example]
assertions: [commands_succeed]

benchmark:
  baseline: baseline
  datasets:
    smoke: example-smoke
  setup:
    command: [example-prepare, "{input}", "{cache}/prepared"]
  warmups: 1
  repeats: 3
  order: balanced
  cache_policy: uncontrolled
  output_extension: .txt
  variables:
    threads: "1"
  versions:
    - name: example
      command: [example, --version]
  variants:
    - id: baseline
      matrix:
        threads: ["1", "2", "4"]
      command: [example, --threads, "{threads}", -o, "{output}", "{input}"]
    - id: optimized
      command: [example, --fast, --threads, "{threads}", -o, "{output}", "{input}"]
  checks:
    - id: decoded-output
      type: stdout-sha256-equal
      command: [example, decode, "{output}"]
```

第一版支持下列占位符：

- `{input}`：数据集下载文件。
- `{output}`：当前 variant 的临时输出。
- `{work}`：本次实验临时目录。
- `{cache}`：guide 数据缓存根目录。
- `{dataset}`、`{tier}`、`{variant}`：当前实验上下文。
- `variables` 中显式声明的值。

命令使用 argv 列表执行，不经过隐式 shell。只有确实需要管道、重定向或复合命令时，案例才应
显式使用 `bash -o pipefail -c ...`。

`setup.command` 在计时前执行一次，适合建立索引或准备可复用分片，并作为 `@setup` 写入
`commands.tsv`。`matrix` 按键名字典序做确定性笛卡尔展开；例如 `baseline` 的
`threads: ["1", "2"]` 展开为 `baseline-threads-1` 和 `baseline-threads-2`。可在命令行覆盖：

```bash
go run ./guide/cmd/bench --matrix threads=1,4,8 --tier medium <case-id>
```

`balanced` 顺序按正式轮次循环移动 variant。例如 A/B/C 三轮依次执行 A-B-C、B-C-A、C-A-B；
warmup 原始数据也写入 `runs.tsv`，但不参与汇总。

## 采集语义

runner 使用 GNU `/usr/bin/time` 的 `%e/%U/%S/%M/%x`：

| 字段 | 含义 | 单位 |
|---|---|---|
| `wall_seconds` | 实际经过时间 | 秒 |
| `user_seconds` | 用户态 CPU 时间 | 秒 |
| `system_seconds` | 内核态 CPU 时间 | 秒 |
| `max_rss_kb` | 最大常驻内存 | KiB |
| `exit_code` | 命令退出码 | 整数 |

`average_cores = (user_seconds + system_seconds) / wall_seconds`。汇总以 wall time 中位数计算相对
baseline 的 speedup，同时保存平均值、范围、总体标准差和 CV。单轮 pilot 的标准差和 CV 为 0，
不能据此声称结果稳定。

## 正确性检查

性能数据只有在所有检查通过后才生成正式汇总。第一版提供
`stdout-sha256-equal`：对每个 variant 执行检查命令，计算标准输出 SHA-256，并与 baseline 比较。
对 VCF 推荐使用：

```yaml
command: [bcftools, view, --no-version, -H, "{output}"]
```

检查发生在所有计时轮次之后，不计入 wall time，也不穿插在 variants 之间改变正式运行顺序。

## 标准产物

结果写入 `.cache/guide-data/benchmarks/<case-id>/<tier>/<run-id>/`：

- `environment.tsv`：数据、版本和执行参数。
- `state.tsv`：实验开始和结束时的负载、可用内存和 CPU 频率。
- `commands.tsv`：展开后的准确命令。
- `runs.tsv`：warmup 和 measured 的逐轮原始值。
- `checks.tsv`：每个 variant 的正确性摘要。
- `summary.tsv`：只基于 measured 数据的派生统计。
- `report.md`：待人工审阅的报告草稿。
- `manifest.sha256`：以上文件的完整性清单。

`runs.tsv` 在每轮完成后立即落盘。收到 SIGINT/SIGTERM 或执行失败时，runner 会终止当前完整
子进程组、清理一次性工作目录，并把 `state.tsv` 标记为 `cancelled` 或 `failed`；已经产生的文件
仍保留在结果目录，并生成部分 `manifest.sha256`，便于判断实验执行到了哪里。中断的结果没有
`summary.tsv` 和正式报告，不能用于性能结论。

设备字段由 runner 自动采集，不写进案例：匿名 `host_id`、内核、CPU 型号、socket/物理核/逻辑
核、总内存、CPU governor，以及输入/输出文件系统、块设备型号和旋转盘标记。动态状态记录
1/5/15 分钟 load average、可用内存和当前 CPU 频率。探测不需要 root；字段不可用时写
`unavailable`，不影响实验执行。

TSV 中的字段换行统一写成字面量 `\n`，保证一条记录只占一个物理行。原始结果和输出不提交
Git；人工审阅后的结论才进入 `guide/reports/`。

## 第一版边界

当前不增加 DAG、集群执行、容器管理或 cgroup 依赖。setup 只允许一条命令，参数矩阵应控制
组合数量；大规模矩阵必须由使用者显式请求，不能作为日常默认值。
