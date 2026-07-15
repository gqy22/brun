<p align="center">
  <img src="docs/assets/brun-logo.svg" width="132" alt="brun logo">
</p>

<h1 align="center">brun</h1>

<p align="center">
  Run bioinformatics commands like <code>nohup</code>, but keep the record.
</p>

<p align="center">
  <a href="https://github.com/biotools/brun/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/biotools/brun/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/biotools/brun/releases"><img alt="Release" src="https://img.shields.io/github/v/release/biotools/brun?include_prereleases&label=release"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white">
  <img alt="License" src="https://img.shields.io/badge/license-MIT-16a34a">
</p>

`brun` 是面向生物信息学开发的运行记录与日志管理工具。它包装任意 shell 命令，自动记录日志、环境、Git 元数据、脚本快照、输出文件、资源占用、标签、备注和复跑信息。

它适合日常生信开发中最常见的场景：完整 workflow 引擎太重，但裸 `nohup`、手写日志和人工记录又不够可靠。

## 核心能力

| 能力 | 说明 |
|------|------|
| 默认后台运行 | 像 `nohup` 一样关闭终端后继续跑，但无需手写重定向 |
| 自动日志归档 | stdout、stderr、命令、环境和 metadata 统一保存 |
| 脚本快照 | 自动保存运行时输入脚本，方便事后追溯 |
| 输出文件检测 | 通过 fs-diff 记录新增、修改、删除的输出文件 |
| 运行检索 | 按 project、status、tag、关键词和时间范围查询历史 |
| Web Dashboard | 浏览器查看运行状态、日志、输出文件和进程资源 |
| 复跑与审计 | 从历史记录恢复命令，保留 Git、环境和资源信息 |
| 内置经验指南 | 离线查看经过验证的生信命令写法、性能建议和常见陷阱 |

## 安装

### 从源码安装

```bash
go install github.com/biotools/brun@latest
```

### 本地构建

```bash
git clone https://github.com/biotools/brun.git
cd brun
make build
./bin/brun --help
```

### 下载 Release

预编译包会随 GitHub Release 发布，目标平台包括：

- Linux amd64 / arm64
- macOS amd64 / arm64

## 快速开始

```bash
# 1. 运行命令，默认后台执行
brun -- bwa mem -t 16 ref.fa reads.fq.gz

# 需要名称、项目、标签等元数据时使用规范入口
brun run -n align-S1 -p wgs -t sample:S1 -- bwa mem -t 16 ref.fa reads.fq.gz

# 2. 查看历史记录
brun list -p wgs

# 3. 查看日志、输出和脚本快照
brun logs --latest --tail 100
brun outputs --latest
brun script --latest
```

启动 Web Dashboard：

```bash
brun web
open http://<host>:9213
```

## 为什么不是 nohup

你以前这样跑脚本：

```bash
nohup bash test.sh > test.sh.o 2> test.sh.er &
# 然后手动记 PID、记命令、猜日志在哪...
```

现在用 brun：

```bash
brun run -n my-script -- bash test.sh
# → [nohup] PID=12345, RunID=20260604-153012-a8f3c2
# → [nohup] stdout: ~/.brun/runs/2026/06/04/20260604-153012-a8f3c2/stdout.o
# → [nohup] stderr: ~/.brun/runs/2026/06/04/20260604-153012-a8f3c2/stderr.er
# → 关掉终端也没事，进程继续跑
```

**区别：**
| | nohup | brun |
|---|---|---|
| 后台运行 | 需要 `&` + 手动管理 | **默认行为**，直接跑 |
| 日志 | 自己指定 `> out 2> err` | 自动记录 stdout + stderr |
| 命令记录 | 没有 | 自动存 command.sh + metadata.yaml |
| 输出文件 | 不知道产生了什么 | fs-diff 自动检测 |
| 查找历史 | 记不清跑了啥 | `brun list` / `brun list -s "关键词"` |

更多例子：

```bash
# 跑生物信息学流程（关掉终端也不中断）
brun run -n align-S1 -p wgs -t rnaseq -- bwa mem -t 16 ref.fa reads.fq.gz

# 跑长时间任务
brun run -n gatk-hc -p variant --timeout 86400 \
    gatk HaplotypeCaller -I input.bam -O output.vcf.gz

# 跑 Python 脚本
brun run -n train-model -p ml -- python3 train.py --epochs 100 --gpu 0
```

跑完随时回来查结果：

```bash
brun list                          # 所有记录
brun list -p wgs                  # 只看 wgs 项目
brun list -s "bwa"                # 搜哪个 run 用了 bwa
brun show --latest                 # 最新一次详情
brun logs --latest --tail 100      # 看最后 100 行日志
brun outputs --latest              # 自动检测到的输出文件
brun script --latest               # 查看运行时保存的脚本快照
```

### 前台运行（调试时用）

```bash
# 加 -f 前台运行，实时看输出（等同普通执行）
brun run -f -n test-align -- bwa mem -t 4 ref.fa reads.fq.gz
```

### 流水线多步骤

```bash
# 每步一个 name + project + 关键 tag
brun run -n step1-map -p pipeline -t workflow:A -- minimap2 ...
brun run -n step2-sort -p pipeline -t workflow:A -- samtools sort ...
brun run -n step3-call -p pipeline -t workflow:A -- bcftools call ...

# 用 tag 把整个流水线串起来
brun list -t workflow:A           # 查看整个流水线的所有步骤
```

### 处理特殊退出码

某些工具用非零退出码表示正常情况（如 `bcftools call` 无变异时返回 1）：

```bash
brun run -n variant-call --allow-exit 1 -- bcftools call -mv ...
brun run -n grep-check --allow-exit 1,2 -- grep "pattern" file.txt
```

### 事后查找（不需要提前打 tag）

```bash
brun list -s "S1"                 # S1 相关的所有运行
brun list -s "ref_v2"              # 哪次用了新参考基因组
brun list -p wgs -s "sort"         # wgs 项目里的排序步骤
brun list --since today            # 今天跑了什么
brun list --since 1w               # 最近一周
brun list -S failed --since today   # 今天失败的
```

### 标签和备注

```bash
brun tag --latest important failed-debug
brun note --latest "STAR index 参数测试"
```

### 复跑

```bash
brun rerun --latest --dry-run      # 先看看会执行什么
brun rerun --latest                # 确认后真正复跑
```

### 终止运行中的任务

```bash
brun stop <run_id>                 # 终止指定任务（SIGTERM + 3s 宽限期）
brun stop --latest                 # 终止最新运行中的任务
brun stop <run_id> -f              # 强制终止（跳过宽限期，直接 SIGKILL）
```

终止机制：
- 向整个进程组发送 `SIGTERM`，等待 3 秒优雅退出
- 超时未退出则升级为 `SIGKILL` 强制结束
- 自动探活：进程已不存在时自动标记为 `failed`
- 终止前记录最终资源数据（Peak RSS / CPU Time）
- CLI 和 Web Dashboard 共用同一套终止逻辑（`cmd.StopRun`）

## Web Dashboard

启动后浏览器访问即可使用完整可视化管理界面：

```bash
brun web                    # 默认监听 0.0.0.0:9213；端口占用时自动寻找后续端口
brun web --port 8080        # 自定义端口；端口占用时直接失败
brun web --addr 127.0.0.1   # 仅本机访问
```

功能概览：

- **Dashboard 首页**：所有运行记录表格视图，支持按项目/状态/标签/关键词过滤，底部统计总运行数/成功率/今日运行数，running 任务自动刷新
- **任务详情页**：左右分栏布局 — 左侧信息面板（状态/耗时/资源消耗/命令/Git/时间），右侧多标签面板（Script / stdout / stderr / **Processes** / 输出文件列表）
- **操作按钮**：终止运行中任务、删除已完成记录、重跑（生成全新 Run 记录）、复制命令
- **资源监控**：每个任务自动记录峰值内存（Peak RSS）和 CPU 累计时间，详情页直接展示；运行中任务可查看实时子进程列表（Processes 标签页）
- **移动端适配**：小屏幕自动切换为卡片列表视图
- **局域网访问**：启动时自动打印所有可用 IP 地址，同局域网任意设备可访问

```bash
# 启动后浏览器打开
open http://localhost:9213

# 或从其他设备访问（手机/平板查看运行状态）
http://192.168.1.x:9213
```

### 资源监控

每个任务执行完毕后自动采集资源数据：

| 指标 | 来源 | 说明 |
|------|------|------|
| Peak Memory | `/proc/{pid}/status` VmHWM | 进程组生命周期峰值物理内存 |
| CPU Time | `/proc/{pid}/stat` utime+stime | 用户态+内核态累计 CPU 时间 |

数据在任务结束时一次性读取，零性能开销。Web 详情页左侧面板和 `brun show` 命令均可查看。

#### 实时进程列表（运行中任务）

对于正在运行的任务（`running` 状态），Web 详情页提供 **Processes** 标签页，实时展示进程组内所有子进程：

| 字段 | 说明 |
|------|------|
| PID / PPID | 进程 ID 和父进程 ID |
| Command | 完整命令行（截断显示，悬停看完整） |
| State | 进程状态（R 运行 / S 睡眠 / D 不可中断等） |
| CPU | 累计 CPU 时间 |
| RSS | 当前物理内存占用（>100MB 标红） |

每 3 秒自动刷新，任务结束后标签页自动隐藏。适用于排查生信流程中哪个步骤在消耗资源。

## 命令一览

| 命令 | 说明 | 常用示例 |
|------|------|----------|
| `brun -- <cmd>` | 快捷运行并完整记录（默认后台） | `brun -- cmd` |
| `brun run -- <cmd>` | 执行并完整记录（默认后台） | `brun run -n job1 -p proj -t tagA -- cmd` |
| `brun run -f -- <cmd>` | 前台运行 | `brun run -f -n job1 -- cmd` |
| `brun list` | 列出运行历史 | `brun list -p proj -s "bwa" --since 1d` |
| `brun show <id>` | 查看详情（含资源数据） | `brun show --latest` |
| `brun script <id>` | 查看脚本快照 | `brun script --latest` |
| `brun logs <id>` | 查看日志 | `brun logs --latest --tail 50 --stderr` |
| `brun outputs <id>` | 查看输出文件 | `brun outputs --latest` |
| `brun diag <id>` | 查看运行诊断 | `brun diag --latest --all` |
| `brun guide` | 查看内置生信命令经验 | `brun guide search "bcftools 并行"` |
| `brun tag <id> TAG...` | 添加标签 | `brun tag --latest sample:S1 production` |
| `brun note <id> "text"` | 添加备注 | `brun note --latest "参数说明"` |
| `brun stop <id>` | 终止运行中的任务 | `brun stop --latest` / `brun stop <id> -f` |
| `brun rerun <id>` | 重新运行 | `brun rerun --latest --dry-run` |
| `brun web` | 启动 Web Dashboard | `brun web --port 8080` |
| `brun init` | 生成 brun.yaml | `brun init my-proj` |
| `brun clean` | 清理旧记录（默认预览） | `brun clean --older-than 30d --write` |

## brun run 参数

```bash
brun run [options] -- <command...>
```

| 参数 | 短参数 | 说明 |
|------|--------|------|
| `--name` | `-n` | run 名称（用于区分同一步骤的不同尝试） |
| `--project` | `-p` | 项目名（自动从 brun.yaml / 目录名推断） |
| `--tag` | `-t` | 标签，支持逗号分隔：`-t align,hg38` 等价于 `-t align -t hg38` |
| `--note` | | 备注文本 |
| `--foreground` | `-f` | 前台运行（默认后台） |
| `--allow-exit` | | 允许的非零退出码 (逗号分隔，如: `1,2,127`) |
| `--no-fs-diff` | | 禁用文件系统自动检测（默认开启） |
| `--timeout` | | 超时时间（秒） |
| `--cwd` | | 指定运行目录 |

### 后台运行机制

brun **默认以 nohup 方式后台运行**：

- 进程独立于终端，关闭 SSH/终端不会中断任务
- 日志统一记录到 `~/.brun/runs/YYYY/MM/DD/<run_id>/`
- 启动后立即返回 PID，不阻塞终端
- 子进程通过 `--foreground` 内部调用确保实际命令被执行

### 输出文件检测

brun **自动**通过文件系统快照 diff 检测输出文件，无需手动声明：

```bash
# 脚本里正常写输出路径就行，brun 自动发现
brun run -n sort-bam -p wgs -- samtools sort -o result.bam aln.sam
# → outputs 自动检测到 result.bam (kind=output, status=created)
```

自动分类规则：
- `.py/.r/.sh/.nf` → script
- `.yaml/.yml/.json/.toml` → config
- `.html/.htm` → report
- `.bam/.cram/.sam/.fastq/.fq` → output/input（根据路径判断）
- `.vcf.gz` → output

## brun list 过滤

```bash
brun list                              # 全部，含 NAME 列
brun list -p wgs                       # 按项目
brun list -S failed                     # 按状态 (-S = --status)
brun list -t sample:S1                  # 按 tag
brun list -s "bwa"                     # 搜索命令/名称中的关键词
brun list --since today                # 今天以来的
brun list --since 1w                   # 最近一周
brun list --until 2026-05-10           # 某日期之前
brun list -p wgs -s "mem" --since 3d    # 组合使用
```

时间过滤只接受 `YYYY-MM-DD`、RFC3339、`today`、`Nh`、`Nd`、`Nw`，格式错误会直接报错。

面向脚本和智能体时，优先使用 JSON 输出：

```bash
brun list --json
brun show --latest --json
brun outputs --latest --json
brun diag --latest --json
brun clean --older-than 30d --json
```

可恢复错误会尽量输出稳定的 `Code` 和下一步 `Hint`：

```text
Error: --since 无效时间 "nope"
Code: invalid_time_filter
Hint: 使用 YYYY-MM-DD、RFC3339、today、Nh、Nd、Nw
```

## 数据存储

默认存储在 `~/.brun/`，可通过 `BRUN_HOME` 环境变量覆盖：

```
~/.brun/
├── db.sqlite              # SQLite 数据库
├── brun.log               # brun 自身日志
└── runs/
    └── YYYY/MM/DD/
        └── <run_id>/
            ├── metadata.yaml     # 结构化元数据
            ├── command.sh        # 完整命令
            ├── script.<name>      # 输入脚本快照（如 script.04.sh）
            ├── stdout.o          # 标准输出
            ├── stderr.er         # 错误输出
            └── env.txt           # 环境摘要
```

## 项目配置 (brun.yaml)

```bash
brun init my-project
```

生成的 `brun.yaml` 可自定义忽略模式等。

## E2E 测试

项目包含完整的生物信息学集成测试套件，覆盖真实工具链：

```bash
# 测试所有真实生物信息学工具的集成
bash e2e/run.sh

# 28 项测试覆盖:
# - Test 1: minimap2 + samtools 排序/flagstat/stats 流水线
# - Test 2: hisat2 短 reads 比对
# - Test 3: FastQC 质控
# - Test 4: bcftools mpileup/call/index/stats 变异检测
# - Test 5: bedtools intersect/coverage 区间分析
# - Test 6: 完整流水线 + tag/note/rerun
# - Test 7: 错误处理 (不存在的命令 / 非 zero exit)
# - Test 8: 并发压力测试 (5 并发)
# - Test 9: 日志查看 (stdout/stderr/tail)
# - Test 10: fs-diff 自动输出检测与分类
```

## 开发

```bash
# 测试
make test

# 编译（带 upx 压缩）
make release

# 交叉编译
make release-linux-amd64
make release-linux-arm64
make release-darwin-arm64
make release-darwin-amd64
make release-all
```

## 技术栈

- **Go 1.25+** / **cobra** CLI
- **SQLite** (modernc.org/sqlite, 纯 Go 无 CGO)
- **YAML** 配置解析
- **WAL 模式 + 指数退避重试** 支持并发写入
- **内置 nohup** 通过进程分离实现默认后台运行
- **Web Dashboard**: Go `net/http` + `embed.FS` 内嵌模板/静态资源，零外部依赖
- **前端**: 原生 HTML/CSS/JS（无框架），DM Sans 字体 + 终端风格日志查看器
- **资源采集**: Linux `/proc/{pid}/` 文件系统读取 VmHWM / utime+stime

## License

MIT
