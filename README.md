# brun

面向生物信息学开发的运行记录与日志管理工具。

在任意目录、任意项目中运行脚本或软件时，自动记录命令、日志、环境、脚本快照、输出文件和运行元数据，便于后续检索、复现、管理和审计。

## 你该如何使用 brun

### 替代 nohup（最常用）

你以前这样跑脚本：

```bash
nohup bash test.sh > test.sh.o 2> test.sh.er &
# 然后手动记 PID、记命令、猜日志在哪...
```

现在用 brun：

```bash
brun run -n my-script -- bash test.sh
# → [detach] PID=12345, 日志: ~/.bio-runner/detach.log
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
brun show latest                   # 最新一次详情
brun logs latest --tail 100        # 看最后 100 行日志
brun outputs latest                # 自动检测到的输出文件
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
brun tag latest important failed-debug
brun note latest "STAR index 参数测试"
```

### 复跑

```bash
brun rerun latest --dry-run        # 先看看会执行什么
brun rerun latest                  # 确认后真正复跑
```

## Web Dashboard

启动后浏览器访问即可使用完整可视化管理界面：

```bash
brun web                    # 默认端口 9313
brun web --port 8080        # 自定义端口
```

功能概览：

- **Dashboard 首页**：所有运行记录表格视图，支持按项目/状态/标签/关键词过滤，底部统计总运行数/成功率/今日运行数，running 任务自动刷新
- **任务详情页**：左右分栏布局 — 左侧信息面板（状态/耗时/资源消耗/命令/Git/时间），右侧终端风格日志查看器（stdout/stderr 切换、搜索高亮、tail 截断、auto-refresh）+ 输出文件列表
- **操作按钮**：终止运行中任务、删除已完成记录、重跑（生成全新 Run 记录）、复制命令
- **资源监控**：每个任务自动记录峰值内存（Peak RSS）和 CPU 累计时间，详情页直接展示
- **移动端适配**：小屏幕自动切换为卡片列表视图
- **局域网访问**：启动时自动打印所有可用 IP 地址，同局域网任意设备可访问

```bash
# 启动后浏览器打开
open http://localhost:9313

# 或从其他设备访问（手机/平板查看运行状态）
http://192.168.1.x:9313
```

### 资源监控

每个任务执行完毕后自动采集资源数据：

| 指标 | 来源 | 说明 |
|------|------|------|
| Peak Memory | `/proc/{pid}/status` VmHWM | 进程生命周期峰值物理内存 |
| CPU Time | `/proc/{pid}/stat` utime+stime | 用户态+内核态累计 CPU 时间 |

数据在任务结束时一次性读取，零性能开销。Web 详情页左侧面板直接展示。

## 命令一览

| 命令 | 说明 | 常用示例 |
|------|------|----------|
| `brun run -- <cmd>` | 执行并完整记录（默认后台） | `brun run -n job1 -p proj -t tagA -- cmd` |
| `brun run -f -- <cmd>` | 前台运行 | `brun run -f -n job1 -- cmd` |
| `brun list` | 列出运行历史 | `brun list -p proj -s "bwa" --since 1d` |
| `brun show <id\|latest>` | 查看详情 | `brun show latest` |
| `brun logs <id\|latest>` | 查看日志 | `brun logs latest --tail 50 --stderr` |
| `brun outputs <id\|latest>` | 查看输出文件 | `brun outputs latest` |
| `brun tag <id> TAG...` | 添加标签 | `brun tag latest sample:S1 production` |
| `brun note <id> "text"` | 添加备注 | `brun note latest "参数说明"` |
| `brun rerun <id\|latest>` | 重新运行 | `brun rerun latest --dry-run` |
| `brun web` | 启动 Web Dashboard | `brun web --port 8080` |
| `brun init` | 生成 brun.yaml | `brun init my-proj` |
| `brun clean` | 清理旧记录 | `brun clean --dry-run` |

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
- 日志统一记录到 `~/.bio-runner/runs/YYYY/MM/DD/<run_id>/`
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

## 数据存储

默认存储在 `~/.bio-runner/`，可通过 `BRUN_HOME` 环境变量覆盖：

```
~/.bio-runner/
├── db.sqlite              # SQLite 数据库
├── detach.log             # 后台运行日志
└── runs/
    └── YYYY/MM/DD/
        └── <run_id>/
            ├── metadata.yaml     # 结构化元数据
            ├── command.sh        # 完整命令
            ├── stdout.log        # 标准输出
            ├── stderr.log        # 错误输出
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
bash test_e2e/run_all.sh

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

- **Go 1.22+** / **cobra** CLI
- **SQLite** (modernc.org/sqlite, 纯 Go 无 CGO)
- **YAML** 配置解析
- **WAL 模式 + 指数退避重试** 支持并发写入
- **内置 nohup** 通过进程分离实现默认后台运行
- **Web Dashboard**: Go `net/http` + `embed.FS` 内嵌模板/静态资源，零外部依赖
- **前端**: 原生 HTML/CSS/JS（无框架），DM Sans 字体 + 终端风格日志查看器
- **资源采集**: Linux `/proc/{pid}/` 文件系统读取 VmHWM / utime+stime

## License

MIT
