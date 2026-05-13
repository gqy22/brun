# brun

面向生物信息学开发的运行记录与日志管理工具。

在任意目录、任意项目中运行脚本或软件时，自动记录命令、日志、环境、脚本快照、输出文件和运行元数据，便于后续检索、复现、管理和审计。

## 快速开始

```bash
# 安装
go install github.com/biotools/brun@latest

# 或本地编译
make build

# 初始化项目（生成 brun.yaml）
cd your-project
brun init

# 运行并记录（推荐用法）
brun run -n align-S1 -p wgs -t rnaseq -- bwa mem -t 16 ref.fa reads.fq.gz

# 查看记录
brun list                          # 所有记录（含 NAME 列）
brun list -p wgs -s "bwa"          # 按项目 + 搜索关键词
brun list -s "ref_v2"              # 哪次用了新参考基因组
brun list --since today            # 今天跑了什么
brun show latest                    # 最新一次详情
brun logs latest --tail 100         # 看最后 100 行日志
brun outputs latest                 # 自动检测到的输出文件

# 标签和备注
brun tag latest important failed-debug
brun note latest "STAR index 参数测试"

# 复跑
brun rerun latest --dry-run
```

## 命令一览

| 命令 | 说明 | 常用示例 |
|------|------|----------|
| `brun run -- <cmd>` | 执行并完整记录 | `brun run -n job1 -p proj -t tagA -- cmd` |
| `brun list` | 列出运行历史 | `brun list -p proj -s "bwa" --since 1d` |
| `brun show <id\|latest>` | 查看详情 | `brun show latest` |
| `brun logs <id\|latest>` | 查看日志 | `brun logs latest --tail 50 --stderr` |
| `brun outputs <id\|latest>` | 查看输出文件 | `brun outputs latest` |
| `brun tag <id> TAG...` | 添加标签 | `brun tag latest sample:S1 production` |
| `brun note <id> "text"` | 添加备注 | `brun note latest "参数说明"` |
| `brun rerun <id\|latest>` | 重新运行 | `brun rerun latest --dry-run` |
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
| `--no-fs-diff` | | 禁用文件系统自动检测（默认开启） |
| `--timeout` | | 超时时间（秒） |
| `--cwd` | | 指定运行目录 |

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

## 推荐工作流

```bash
# 1. 日常运行 — 只需 name + project + 关键 tag
brun run -n align-S1 -p wgs -t sample:S1 -- bwa mem -t 16 ref.fa S1.fq.gz

# 2. 事后查找 — 不需要提前打 tag，靠搜索
brun list -s "S1"                      # S1 相关的所有运行
brun list -s "ref_v2"                   # 哪次用了新参考基因组
brun list -p wgs -s "sort"             # wgs 项目里的排序步骤

# 3. 时间范围
brun list --since today -S failed        # 今天失败的
brun list -p rnaseq --since 1w           # 本周 RNA-seq 的运行

# 4. 长时间任务后台运行
nohup brun run -n gatk-hc -p variant --timeout 86400 \
    gatk HaplotypeCaller -I input.bam -O output.vcf.gz > /dev/null 2>&1 &

# 5. 流水线多步骤
brun run -n step1-map -p pipeline -t workflow:A -- minimap2 ...
brun run -n step2-sort -p pipeline -t workflow:A -- samtools sort ...
brun list -t workflow:A                                   # 查看整个流水线
```

## 数据存储

默认存储在 `~/.bio-runner/`，可通过 `BRUN_HOME` 环境变量覆盖：

```
~/.bio-runner/
├── db.sqlite              # SQLite 数据库
└── runs/
    └── YYYY/MM/DD/
        └── <run_id>/
            ├── metadata.yaml     # 结构化元数据
            ├── command.sh        # 完整命令
            ├── stdout.log        # 标准输出
            ├── stderr.log        # 错误输出
            ├── env.txt           # 环境摘要
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

## License

MIT
