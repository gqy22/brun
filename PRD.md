# brun：面向生物信息学开发的运行记录与日志管理工具 PRD

## 1. 文档信息

**产品名称**：brun
**开发语言**：Go
**产品形态**：跨项目命令行工具，后续可扩展 Web UI / 本地服务
**目标用户**：生物信息学开发者、算法工程师、科研人员、小型生信团队
**核心目标**：在任意目录、任意项目中运行脚本或软件时，自动记录命令、日志、环境、脚本快照、输出文件和运行元数据，便于后续检索、复现、管理和审计。

---

## 2. 背景与问题

生物信息学开发过程中，经常需要反复运行大量脚本和外部软件，例如：

* Python / R / Shell 脚本；
* bwa、samtools、bcftools、STAR、featureCounts、FastQC、MultiQC 等生信工具；
* Snakemake / Nextflow 工作流；
* 临时测试脚本、参数实验和数据处理脚本。

实际开发中通常存在以下问题：

1. **运行目录分散**
   用户会在多个项目、多个文件夹中开发和运行脚本，日志分散在不同目录，长期难以追踪。

2. **脚本命名相似**
   不同项目中可能都有 `analyze.py`、`run.sh`、`plot.R`、`test.py` 等文件，单靠文件名无法判断一次运行属于哪个项目、哪个版本。

3. **日志管理混乱**
   有些日志在终端滚动输出，有些重定向到文件，有些被覆盖，有些根本没有保存。

4. **难以复现**
   运行后常常无法确认当时的命令参数、Git commit、环境、脚本内容、配置文件和输入输出文件。

5. **输出文件难以追踪**
   运行一次脚本可能生成多个结果文件，后续很难知道某个结果是由哪条命令、哪个脚本、哪个参数产生的。

6. **正式 workflow 与日常开发之间存在断层**
   Snakemake / Nextflow 适合成熟流程，但开发阶段大量临时命令、调试脚本、实验运行不一定适合立即 workflow 化。

因此需要一个更工程化的、跨项目的、轻量级运行记录系统。

---

## 3. 产品定位

brun 是一个面向生物信息学开发的 **全局运行登记系统**。

用户不再直接运行：

```bash
python scripts/analyze.py --sample S1
bwa mem ref.fa reads.fq.gz
Rscript plot.R
```

而是使用：

```bash
brun run -- python scripts/analyze.py --sample S1
brun run -- bwa mem ref.fa reads.fq.gz
brun run -- Rscript plot.R
```

brun 会为每次运行生成唯一 `run_id`，并自动记录：

* 当前工作目录；
* 项目名称；
* 完整命令；
* stdout / stderr 日志；
* 运行开始时间、结束时间、耗时；
* 退出码和状态；
* Git repo、commit、dirty 状态、diff；
* Conda / PATH / 系统环境摘要；
* 脚本和配置文件快照；
* 输入和输出文件元数据；
* 自动发现的新增 / 修改文件；
* 用户自定义 tag、name、note。

brun 不替代 Snakemake / Nextflow，而是补充其在日常开发、临时运行、调试实验中的空白。

---

## 4. 产品目标

### 4.1 核心目标

1. **跨项目统一记录运行历史**
   无论用户在哪个目录执行命令，记录都统一进入 `~/.bio-runner/`。

2. **每次运行可追溯**
   用户可以通过 `run_id` 查到命令、日志、环境、脚本版本和输出文件。

3. **降低用户使用成本**
   使用方式尽量接近原生命令，只需在前面加 `brun run --`。

4. **适配生信大文件场景**
   对 BAM、CRAM、FASTQ、VCF 等大文件默认只记录元数据，不默认复制，避免存储爆炸。

5. **支持长期运行管理**
   能支撑每天几十次运行、连续数年的记录规模。

6. **具备工程扩展能力**
   后续可扩展 Web UI、TUI、workflow 集成、MLflow / DVC 集成、系统调用级文件追踪。

### 4.2 非目标

第一阶段不追求：

* 替代 Snakemake / Nextflow；
* 实现完整任务调度系统；
* 实现远程集群任务管理；
* 实现类似 Grafana / Loki 的集中日志平台；
* 自动解析所有生信软件的参数语义；
* 默认复制所有输出文件；
* 默认对超大文件计算 hash。

---

## 5. 目标用户与使用场景

### 5.1 目标用户

1. **个人生信开发者**
   经常在多个项目目录中运行脚本、调试参数、分析数据。

2. **小型生信团队**
   需要统一规范运行记录，但不想引入重型平台。

3. **科研人员**
   希望保留实验过程，方便论文结果复现。

4. **算法 / pipeline 开发者**
   经常在正式 workflow 形成前进行大量临时运行。

### 5.2 典型场景

#### 场景 1：运行单个脚本

```bash
brun run -- python scripts/count_reads.py --input data/S1.fastq.gz
```

用户之后可以查看：

```bash
brun logs latest
brun show latest
brun outputs latest
```

#### 场景 2：运行外部生信软件

```bash
brun run -- samtools sort -o results/S1.bam tmp/S1.sam
```

brun 记录 stdout、stderr、命令、项目、输出文件 `results/S1.bam`。

#### 场景 3：运行多个同名脚本

不同项目下都有：

```text
scripts/analyze.py
```

brun 通过 `cwd`、Git repo、project、run_id 区分每次运行。

#### 场景 4：捕获运行输出文件

```bash
brun run \
  --output results/S1.bam \
  --output results/S1.bam.bai \
  -- python scripts/align.py --sample S1
```

也可以通过 `brun.yaml` 自动捕获：

```yaml
capture:
  outputs:
    - "results/**/*.bam"
    - "results/**/*.bai"
    - "reports/**/*.html"
```

#### 场景 5：正式 workflow 外层记录

```bash
brun run -- snakemake --use-conda --cores 16
brun run -- nextflow run main.nf -profile conda
```

即使 workflow 本身有日志，brun 也能作为全局运行索引。

---

## 6. 产品形态

### 6.1 第一阶段形态

本地 CLI 工具：

```bash
brun <command>
```

核心数据存储：

```text
~/.bio-runner/
  db.sqlite
  runs/
    2026/
      05/
        13/
          20260513-153012-a8f3c2/
            metadata.yaml
            command.sh
            stdout.log
            stderr.log
            env.txt
            git.diff
            git.patch
            outputs.json
            exit_code.txt
```

### 6.2 后续形态

后续可扩展：

```bash
brun serve
```

提供本地 Web UI：

* Runs；
* Projects；
* Logs；
* Outputs；
* Failed Runs；
* Tags；
* Git diff；
* Artifacts。

---

## 7. 核心概念

### 7.1 Run

一次由 brun 包装执行的命令称为一个 run。

每个 run 拥有全局唯一 `run_id`。

示例：

```text
20260513-153012-a8f3c2
```

### 7.2 Project

project 用于区分不同开发项目。

识别优先级：

1. 命令行参数 `--project`；
2. 当前目录或父目录中的 `brun.yaml`；
3. Git repo 名称；
4. 当前目录名。

### 7.3 Artifact

artifact 是与某次 run 相关的文件记录，包括：

* output；
* input；
* log；
* script；
* config；
* report；
* unknown。

### 7.4 Metadata

metadata 是一次 run 的结构化描述，写入 SQLite，同时可导出为 `metadata.yaml`。

---

## 8. 功能需求

## 8.1 初始化

### 命令

```bash
brun init
```

### 功能

在当前项目生成 `brun.yaml`。

示例：

```yaml
project: rnaseq-dev

capture:
  scripts:
    - "scripts/**/*.py"
    - "scripts/**/*.R"
    - "*.sh"
    - "Snakefile"
    - "*.nf"
  configs:
    - "configs/**/*.yaml"
    - "configs/**/*.yml"
    - "samples.tsv"
  outputs:
    - "results/**/*"
    - "reports/**/*"

ignore:
  - ".git/**"
  - ".snakemake/**"
  - ".nextflow/**"
  - "work/**"
  - "tmp/**"
  - "__pycache__/**"
  - ".ipynb_checkpoints/**"
  - "*.tmp"
  - "*.swp"
  - ".DS_Store"

artifacts:
  copy:
    - "reports/**/*.html"
    - "results/**/*.tsv"
    - "plots/**/*.png"
  link:
    - "results/**/*.bam"
    - "results/**/*.cram"
    - "results/**/*.vcf.gz"
  metadata_only:
    - "data/**/*.fastq.gz"
    - "data/**/*.fq.gz"
```

---

## 8.2 运行命令

### 命令

```bash
brun run -- <command...>
```

### 示例

```bash
brun run -- python scripts/analyze.py --sample S1
brun run -- samtools sort -o results/S1.bam tmp/S1.sam
brun run -- Rscript scripts/plot.R
```

### 参数

```bash
brun run [options] -- <command...>
```

| 参数                   | 说明                   |
| -------------------- | -------------------- |
| `--name NAME`        | 为 run 指定可读名称         |
| `--project PROJECT`  | 手动指定项目名              |
| `--tag TAG`          | 添加 tag，可重复           |
| `--note TEXT`        | 添加备注                 |
| `--input PATH/GLOB`  | 显式声明输入文件             |
| `--output PATH/GLOB` | 显式声明输出文件             |
| `--capture-root DIR` | 限定文件系统 diff 扫描目录，可重复 |
| `--no-fs-diff`       | 禁用运行前后文件系统 diff      |
| `--hash-outputs`     | 对输出文件计算 hash         |
| `--copy-outputs`     | 复制匹配的小型输出文件到 run 目录  |
| `--timeout DURATION` | 设置超时时间               |
| `--cwd DIR`          | 指定运行目录               |

### 行为

执行前：

1. 生成 run_id；
2. 创建 run_dir；
3. 记录 cwd、hostname、user、时间；
4. 识别 project；
5. 收集 Git 信息；
6. 保存 command.sh；
7. 读取 brun.yaml；
8. 捕获脚本 / 配置文件快照；
9. 根据配置和参数扫描输出目录快照。

执行中：

1. 使用 `os/exec` 执行命令；
2. stdout 写入终端和 `stdout.log`；
3. stderr 写入终端和 `stderr.log`；
4. 记录进程退出码。

执行后：

1. 记录结束时间和耗时；
2. 根据退出码标记 `success` 或 `failed`；
3. 再次扫描输出目录；
4. 对比 before / after 文件快照；
5. 记录新增、修改、删除文件；
6. 展开显式 output glob；
7. 记录 artifact metadata；
8. 根据配置复制 / 链接 / 仅记录文件；
9. 写入 SQLite；
10. 写入 `metadata.yaml`。

---

## 8.3 查看运行列表

### 命令

```bash
brun list
```

### 参数

```bash
brun list [options]
```

| 参数                  | 说明         |          |       |
| ------------------- | ---------- | -------- | ----- |
| `--project PROJECT` | 按项目过滤      |          |       |
| `--status success   | failed     | running` | 按状态过滤 |
| `--tag TAG`         | 按 tag 过滤   |          |       |
| `--limit N`         | 限制数量，默认 20 |          |       |
| `--since DURATION`  | 查看最近一段时间   |          |       |
| `--json`            | JSON 输出    |          |       |

### 示例输出

```text
RUN ID                   PROJECT        STATUS    DURATION   COMMAND
20260513-153012-a8f3c2   rnaseq-dev     success   12m33s     bwa mem -t 16 ...
20260513-151102-b91a0e   variant-test   failed    3s         python parse.py ...
```

---

## 8.4 查看运行详情

### 命令

```bash
brun show <run_id|latest>
```

### 输出信息

* run_id；
* name；
* project；
* status；
* command；
* cwd；
* started_at；
* ended_at；
* duration；
* exit_code；
* Git repo；
* Git commit；
* Git dirty；
* stdout / stderr 路径；
* outputs 数量；
* tags；
* note。

---

## 8.5 查看日志

### 命令

```bash
brun logs <run_id|latest>
```

### 参数

| 参数         | 说明         |
| ---------- | ---------- |
| `--stdout` | 只看 stdout  |
| `--stderr` | 只看 stderr  |
| `--tail N` | 只看最后 N 行   |
| `--follow` | 持续跟踪日志     |
| `--open`   | 用默认编辑器打开日志 |

### 示例

```bash
brun logs latest --tail 100
brun logs 20260513-153012-a8f3c2 --stderr
```

---

## 8.6 查看输出文件

### 命令

```bash
brun outputs <run_id|latest>
```

### 输出示例

```text
Run ID: 20260513-153012-a8f3c2
Project: rnaseq-dev

KIND      STATUS     SIZE       PATH
output    created    8.4 GB     results/S1.bam
output    created    3.2 MB     results/S1.bam.bai
output    created    1.1 MB     reports/S1.html
```

### 参数

| 参数             | 说明      |        |        |         |       |
| -------------- | ------- | ------ | ------ | ------- | ----- |
| `--json`       | JSON 输出 |        |        |         |       |
| `--kind output | input   | script | config | report` | 按类型过滤 |
| `--created`    | 只看新增文件  |        |        |         |       |
| `--modified`   | 只看修改文件  |        |        |         |       |

---

## 8.7 重新运行

### 命令

```bash
brun rerun <run_id|latest>
```

### 行为

默认在原始 cwd 中重新执行原命令。

参数：

| 参数                 | 说明             |
| ------------------ | -------------- |
| `--cwd DIR`        | 使用新的运行目录       |
| `--dry-run`        | 只打印命令，不执行      |
| `--with-same-tags` | 继承原 run 的 tags |
| `--name NAME`      | 指定新 run 名称     |

---

## 8.8 标记和备注

### 命令

```bash
brun tag <run_id|latest> TAG...
brun note <run_id|latest> "text"
```

### 示例

```bash
brun tag latest rnaseq failed-debug important
brun note latest "STAR index 参数测试，内存占用偏高"
```

---

## 8.9 清理与压缩

长期使用时，日志和 artifact 会增长。需要内置清理策略。

### 命令

```bash
brun clean [options]
```

### 参数

| 参数                            | 说明             |
| ----------------------------- | -------------- |
| `--older-than 90d`            | 清理早于指定时间的 run  |
| `--compress-logs`             | 压缩日志           |
| `--truncate-large-logs 100MB` | 裁剪超大日志         |
| `--keep-failed`               | 保留失败 run 的完整日志 |
| `--keep-tag TAG`              | 保留指定 tag 的 run |
| `--dry-run`                   | 只显示将执行的操作      |

### 推荐默认策略

1. 最近 30 天：保留完整日志；
2. 30 天到 1 年：压缩 stdout / stderr；
3. 超过 1 年：默认只保留失败 run、important tag、metadata、小型报告；
4. 大文件 artifact 默认只保留路径和 metadata。

---

## 9. 输出文件捕获策略

输出捕获是 brun 的关键能力。

### 9.1 捕获层级

第一阶段采用三层策略：

1. **显式声明输出**
   用户通过 `--output` 指定，可靠性最高。

2. **项目配置捕获**
   通过 `brun.yaml` 的 `capture.outputs` 声明输出模式。

3. **运行前后文件系统 diff**
   在指定 capture root 下扫描 before / after，发现新增、修改、删除文件。

### 9.2 不推荐第一版实现系统调用级追踪

系统调用级追踪如 `strace` 能更精确地捕获文件写入，但存在问题：

* Linux only；
* 依赖 ptrace 权限；
* HPC 环境可能禁用；
* 性能开销较大；
* 解析复杂；
* 跨平台困难。

因此第一版只预留接口，不默认实现。

后续高级模式：

```bash
brun run --trace-fs -- python script.py
```

### 9.3 文件 diff 规则

运行前扫描：

```text
before_snapshot
```

运行后扫描：

```text
after_snapshot
```

比较规则：

* `created`：after 存在，before 不存在；
* `modified`：before 和 after 都存在，但 size 或 mtime 改变；
* `deleted`：before 存在，after 不存在。

### 9.4 默认忽略规则

```yaml
ignore:
  - ".git/**"
  - ".snakemake/**"
  - ".nextflow/**"
  - "work/**"
  - "tmp/**"
  - "__pycache__/**"
  - ".ipynb_checkpoints/**"
  - ".cache/**"
  - "*.tmp"
  - "*.swp"
  - ".DS_Store"
```

### 9.5 大文件策略

生信场景中大文件很多，默认策略为：

| 文件类型                                       | 默认策略             |
| ------------------------------------------ | ---------------- |
| FASTQ / BAM / CRAM / VCF.gz                | 只记录 metadata，不复制 |
| HTML / TSV / CSV / JSON / YAML / PNG / PDF | 可复制              |
| stdout / stderr                            | 永久记录，后续可压缩       |
| 脚本 / 配置                                    | 复制快照             |

大文件记录字段：

* path；
* absolute_path；
* size_bytes；
* mtime；
* kind；
* status；
* optional sha256。

---

## 10. 存储设计

## 10.1 全局目录

默认目录：

```text
~/.bio-runner/
```

可通过环境变量覆盖：

```bash
BRUN_HOME=/path/to/brun-home
```

目录结构：

```text
~/.bio-runner/
  db.sqlite
  runs/
    YYYY/
      MM/
        DD/
          <run_id>/
            metadata.yaml
            command.sh
            stdout.log
            stderr.log
            env.txt
            git.diff
            git.patch
            outputs.json
            artifacts/
```

---

## 10.2 SQLite 表设计

### runs

```sql
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  name TEXT,
  project TEXT,
  cwd TEXT NOT NULL,
  command TEXT NOT NULL,
  status TEXT NOT NULL,
  exit_code INTEGER,
  started_at TEXT NOT NULL,
  ended_at TEXT,
  duration_ms INTEGER,
  run_dir TEXT NOT NULL,
  hostname TEXT,
  username TEXT,
  shell TEXT,
  git_repo TEXT,
  git_branch TEXT,
  git_commit TEXT,
  git_dirty INTEGER DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runs_started_at ON runs(started_at);
CREATE INDEX IF NOT EXISTS idx_runs_project ON runs(project);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
```

### artifacts

```sql
CREATE TABLE IF NOT EXISTS artifacts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  status TEXT,
  path TEXT NOT NULL,
  abs_path TEXT,
  stored_path TEXT,
  size_bytes INTEGER,
  sha256 TEXT,
  mtime TEXT,
  capture_method TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY(run_id) REFERENCES runs(id)
);

CREATE INDEX IF NOT EXISTS idx_artifacts_run_id ON artifacts(run_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_kind ON artifacts(kind);
CREATE INDEX IF NOT EXISTS idx_artifacts_path ON artifacts(path);
```

### tags

```sql
CREATE TABLE IF NOT EXISTS tags (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  tag TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(run_id) REFERENCES runs(id)
);

CREATE INDEX IF NOT EXISTS idx_tags_run_id ON tags(run_id);
CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);
```

### notes

```sql
CREATE TABLE IF NOT EXISTS notes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  note TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(run_id) REFERENCES runs(id)
);
```

### events

用于记录 run 生命周期事件。

```sql
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  message TEXT,
  created_at TEXT NOT NULL,
  FOREIGN KEY(run_id) REFERENCES runs(id)
);
```

---

## 11. Go 技术方案

### 11.1 推荐技术栈

| 模块     | 推荐                                              |
| ------ | ----------------------------------------------- |
| Go 版本  | Go 1.22+                                        |
| CLI    | cobra                                           |
| SQLite | modernc.org/sqlite，避免 CGO                       |
| 配置     | gopkg.in/yaml.v3                                |
| 表格输出   | olekukonko/tablewriter 或 charmbracelet/lipgloss |
| 日志输出   | Go 标准库 + 可选 slog                                |
| 文件匹配   | doublestar 或 filepath.Glob                      |
| Git 信息 | 优先调用 git CLI，后续可选 go-git                        |
| Web UI | 后续 net/http / chi / templ                       |

### 11.2 项目结构

```text
brun/
  go.mod
  main.go
  cmd/
    root.go
    init.go
    run.go
    list.go
    show.go
    logs.go
    outputs.go
    rerun.go
    clean.go
    tag.go
    note.go
  internal/
    runner/
      runner.go
      command.go
    store/
      store.go
      migrations.go
      models.go
    config/
      config.go
    project/
      detect.go
    gitinfo/
      git.go
    capture/
      snapshot.go
      diff.go
      artifact.go
    ids/
      ids.go
    paths/
      paths.go
    logs/
      tail.go
    clean/
      clean.go
  tests/
  README.md
```

### 11.3 核心模块职责

#### cmd

负责 CLI 命令解析。

#### runner

负责执行外部命令、捕获 stdout / stderr、返回退出码。

#### store

负责 SQLite 初始化、迁移、增删查改。

#### config

负责读取 `brun.yaml`。

#### project

负责识别项目名和项目根目录。

#### gitinfo

负责获取 Git repo、branch、commit、dirty、diff。

#### capture

负责文件扫描、diff、artifact 分类、hash、复制 / 链接。

#### clean

负责日志压缩、裁剪和清理策略。

---

## 12. 关键流程设计

### 12.1 brun run 流程

```text
用户执行 brun run
  -> 解析 CLI 参数
  -> 识别 cwd 和 project
  -> 生成 run_id
  -> 创建 run_dir
  -> 初始化数据库记录，status=running
  -> 读取 brun.yaml
  -> 收集 Git 信息
  -> 保存 command.sh
  -> 保存环境摘要
  -> 捕获脚本和配置快照
  -> 扫描 before_snapshot
  -> 执行命令
  -> 实时写 stdout.log / stderr.log
  -> 获取 exit_code
  -> 扫描 after_snapshot
  -> diff 文件变化
  -> 记录 artifacts
  -> 写 metadata.yaml
  -> 更新数据库 status
  -> 打印运行摘要
```

### 12.2 status 判断

| 条件             | status      |
| -------------- | ----------- |
| 命令正在执行         | running     |
| exit_code = 0  | success     |
| exit_code != 0 | failed      |
| brun 自身错误导致未执行 | error       |
| 用户中断           | interrupted |

### 12.3 中断处理

当用户 Ctrl+C：

1. brun 转发信号给子进程；
2. 等待子进程退出；
3. 记录 status=interrupted；
4. 保存已有日志；
5. 更新 SQLite。

---

## 13. 日志设计

### 13.1 日志文件

每次 run 默认生成：

```text
stdout.log
stderr.log
```

可选生成：

```text
combined.log
```

### 13.2 日志写入方式

执行时使用 `io.MultiWriter`：

* stdout 同时写终端和文件；
* stderr 同时写终端和文件。

### 13.3 日志规模估计

假设每天 30 次运行，连续 2 年：

```text
30 × 365 × 2 = 21,900 次运行
```

按单次日志大小估算：

| 每次运行日志 |     两年总量 |
| -----: | -------: |
| 100 KB | 约 2.2 GB |
|   1 MB |  约 22 GB |
|  10 MB | 约 219 GB |
|  50 MB | 约 1.1 TB |
| 100 MB | 约 2.2 TB |

设计目标：

* SQLite 支撑至少 10 万 run 记录；
* 日志文件允许增长到百 GB 级别；
* 支持压缩和清理；
* 大文件 artifact 不默认复制。

### 13.4 日志清理策略

默认建议：

* 最近 30 天完整保留；
* 30 天后压缩；
* 超过 1 年可裁剪成功 run 的大日志；
* 失败 run 默认保留完整日志；
* important tag 永久保留。

---

## 14. 安装与分发

### 14.1 安装方式

第一阶段：

```bash
go install github.com/<org>/brun@latest
```

后续：

```bash
curl -L https://.../brun-linux-amd64 -o brun
chmod +x brun
```

### 14.2 平台支持

第一阶段优先：

* Linux x86_64；
* macOS arm64 / x86_64。

后续支持：

* Windows；
* HPC 环境；
* 容器环境。

---

## 15. MVP 范围

### 15.1 必须实现

1. `brun init`；
2. `brun run -- <command>`；
3. stdout / stderr 捕获；
4. run_id 生成；
5. `~/.bio-runner` 目录管理；
6. SQLite 初始化和 runs 表；
7. `brun list`；
8. `brun show`；
9. `brun logs`；
10. Git commit / dirty 状态记录；
11. `brun.yaml` 读取；
12. `--output` 显式输出记录；
13. capture root 的 before / after diff；
14. `brun outputs`；
15. 基础清理命令 `brun clean --dry-run`。

### 15.2 MVP 暂不实现

1. Web UI；
2. 系统调用级文件追踪；
3. 远程同步；
4. 多用户权限；
5. 完整 metrics 系统；
6. workflow 深度解析；
7. 自动识别所有生信软件参数。

---

## 16. 版本规划

### v0.1：基础可用

* run / list / show / logs；
* SQLite；
* stdout / stderr；
* run_id；
* command.sh；
* metadata.yaml。

### v0.2：项目配置与输出捕获

* brun.yaml；
* project 识别；
* --output；
* capture-root；
* 文件系统 diff；
* outputs 命令。

### v0.3：脚本和环境快照

* scripts 快照；
* configs 快照；
* Git diff；
* env.txt；
* Conda 环境摘要。

### v0.4：查询与清理

* tag / note；
* search；
* clean；
* compress logs；
* truncate large logs。

### v0.5：Web UI / Server

* brun serve；
* Runs 页面；
* Logs 页面；
* Outputs 页面；
* Failed runs 页面。

### v1.0：稳定版

* 完整 CLI；
* 稳定 SQLite schema；
* 文档完善；
* 跨平台 release；
* 测试覆盖；
* 数据迁移机制稳定。

---

## 17. 非功能需求

### 17.1 性能

* `brun run` 启动额外开销应小于 300ms；
* 文件系统 diff 应支持通过 `--capture-root` 限定范围；
* 默认不对大文件计算 sha256；
* 日志写入应为流式写入，不应全部加载到内存。

### 17.2 可靠性

* 即使命令失败，也必须保存日志和 metadata；
* 即使 brun 在执行后阶段出错，也应尽量保留 run_dir；
* SQLite 写入应使用事务；
* 中断时应更新 run 状态。

### 17.3 可维护性

* CLI 层与业务逻辑分离；
* 数据库迁移集中管理；
* 文件捕获模块独立；
* 后续可替换 SQLite 为 PostgreSQL。

### 17.4 安全性

* 不默认上传任何数据；
* 默认本地存储；
* 记录环境变量时使用白名单；
* 避免保存 token、password、secret；
* 支持配置敏感字段过滤。

---

## 18. 风险与对策

### 风险 1：日志量过大

对策：

* 支持压缩；
* 支持裁剪；
* 支持按 tag 保留；
* 超大日志给 warning。

### 风险 2：输出文件误捕获

对策：

* 支持 ignore；
* 支持 capture-root；
* 显式 output 优先；
* diff 结果标记为 `capture_method=fs_diff`。

### 风险 3：生信大文件导致存储爆炸

对策：

* 默认不复制大文件；
* 只记录 metadata；
* 用户通过配置决定 copy / link / metadata_only。

### 风险 4：命令参数冲突

对策：

* 第一版强制推荐使用 `--` 分隔 brun 参数和目标命令。

### 风险 5：跨平台行为不一致

对策：

* 第一阶段优先 Linux/macOS；
* Windows 后续支持；
* 系统调用追踪作为可选高级功能。

---

## 19. 示例用户体验

### 19.1 初始化项目

```bash
cd ~/projects/rnaseq-dev
brun init
```

### 19.2 运行脚本

```bash
brun run --name count-S1 --tag rnaseq -- python scripts/count_reads.py --sample S1
```

输出：

```text
Run started: 20260513-153012-a8f3c2
Project: rnaseq-dev
Logs: ~/.bio-runner/runs/2026/05/13/20260513-153012-a8f3c2/

Command finished successfully in 2m31s
Outputs detected: 3
```

### 19.3 查看运行历史

```bash
brun list --project rnaseq-dev
```

### 19.4 查看日志

```bash
brun logs latest --tail 100
```

### 19.5 查看输出

```bash
brun outputs latest
```

### 19.6 复跑

```bash
brun rerun latest
```

---

## 20. 开发优先级建议

建议按以下顺序开发：

1. 建立 Go module 和 cobra CLI；
2. 实现 run_id 生成；
3. 实现 `~/.bio-runner` 目录管理；
4. 实现 SQLite schema 和 migration；
5. 实现 `brun run` 基础命令执行；
6. 实现 stdout / stderr 捕获；
7. 实现 `brun list`；
8. 实现 `brun show`；
9. 实现 `brun logs`；
10. 实现 Git 信息记录；
11. 实现 `brun.yaml`；
12. 实现显式 `--output`；
13. 实现文件系统 diff；
14. 实现 `brun outputs`；
15. 实现 tag / note；
16. 实现 clean / compress；
17. 编写 README 和使用示例；
18. 添加测试；
19. 做 release 二进制。

---

## 21. 一句话总结

brun 是一个用 Go 开发的、面向生物信息学开发场景的跨项目运行记录工具。它通过统一的 CLI 包装任意命令，自动保存日志、命令、环境、Git 信息、脚本快照和输出文件索引，让用户在长期、高频、多项目开发中依然能够清楚地知道：

> 我什么时候，在什么目录，用什么脚本、什么参数、什么环境，生成了哪些结果文件，以及当时的日志是什么。

