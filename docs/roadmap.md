# Roadmap

## 已完成的架构治理

### 背景

项目早期已经有 `cmd/`、`internal/` 和 `web/` 目录，但根目录 `main.go` 曾经承担了过多职责：

- 程序入口。
- Cobra root command 和子命令装配。
- 多个子命令的具体实现。
- 运行编排、查询、日志输出和 Web 启动逻辑。
- `web/templates` 与 `web/static` 的资源嵌入。

随着后续要加入任务调度、依赖运行、daemon 和更完整的 Web 操作，继续把核心 CLI 编排放在根目录会让入口层越来越难维护。因此已先完成第一阶段项目结构治理，让入口、命令层和 Web 资源边界更清楚。

### 当前目录结构

当前已调整为：

```text
cmd/
  brun/
    main.go
internal/
  cli/
    root.go
    run.go
    query.go
    manage.go
  capture.go
  git.go
  logger.go
  store.go
web/
  embed.go
  templates/
  static/
```

职责划分：

- `cmd/brun/main.go`：只做程序入口，读取 build-time version 信息，调用 `cli.Execute(...)`。
- `internal/cli`：负责 Cobra 命令定义、参数解析、帮助模板和命令间的用户交互输出。
- `internal/store.go`：负责数据库模型、迁移和持久化操作。
- `internal/capture.go`：负责配置解析、文件快照和 artifact 分类。
- `internal/git.go`：负责项目识别、Git 信息采集和 run id/path 生成。
- `internal/logger.go`：负责日志初始化和全局 logger。
- `web/embed.go`：集中处理 `go:embed`，避免 `cmd/brun/main.go` 需要跨目录嵌入资源。

### 已完成事项

第一阶段已经完成以下低风险搬迁：

1. 新增 `web` package，把模板和静态资源嵌入从根目录入口移走。
2. 新增 `internal/cli`，把 Cobra root command、help/usage 模板和各个命令函数迁入。
3. 将根目录 `main.go` 移到 `cmd/brun/main.go`，保持入口层薄。
4. 将 `internal/cli` 按职责拆为 `root.go`、`run.go`、`query.go` 和 `manage.go`。
5. 保持已有行为、命令名称、flag、输出格式和测试语义不变。

### 验收标准

- [x] 根目录不再有 `main.go`。
- [x] `cmd/brun/main.go` 少于 50 行，只负责入口和 build-time 变量。
- [x] Cobra 命令定义集中在 `internal/cli`。
- [x] Web 资源嵌入集中在 `web/embed.go`。
- [x] `go test ./...` 通过。
- [x] `go run ./cmd/brun --help`、`go run ./cmd/brun run --help`、`go run ./cmd/brun web --help` 行为保持一致。
- [x] 后续调度功能可以在新的执行编排层中新增，不需要继续扩大入口层。

### 下一步方向

短期不急着拆 `internal/store.go`、`internal/capture.go`、`internal/git.go` 和 `internal/logger.go`。这些文件职责单一、体量较小，继续拆成子包会带来较多 import churn，但收益有限。

当前架构已经够用一段时间。下一轮优先级不应继续做结构拆分，而应转向 `docs/fallbacks.md` 中的行为清理，先减少会造成数据丢失、用户无感或行为漂移的 fallback。

运行诊断已经开始落地：`brun run` 会把推断行为和关键写入失败记录到 run 目录下的 `diagnostics.jsonl`，并在前台运行结束时输出 warning 摘要。下一步重点不是继续拆包，而是把这些诊断接入 list/web，使用户不需要进入 run 目录也能看到“成功但有提示”的状态。

SQLite 当前定位为快速索引层，run 目录中的 `command.sh`、`stdout.o`、`stderr.er`、`metadata.yaml` 和 `diagnostics.jsonl` 是主审计载体。默认 `BRUN_SQLITE_SYNC=off` 用于避免短任务被 SQLite 同步拖慢；需要更强写入一致性时可以设置 `BRUN_SQLITE_SYNC=normal` 或 `BRUN_SQLITE_SYNC=full`。如果索引缺失或损坏，使用 `brun repair-index --write` 从 run 目录重建缺失的 run 记录。

建议优先级：

1. 修复后台 detached run 的 RunID 不一致问题。
2. 给 run 记录增加诊断摘要字段，例如 warning 数、最后诊断时间和关键 code。
3. 在 `brun list` 和 Web run detail 中展示诊断状态。
4. 提供索引修复能力，从 run 目录重建 SQLite 中缺失的 run 记录。
5. 逐步把关键审计文件、tag/note/artifact/resource/metadata 的写入失败从“仅提示”收紧为明确状态。
6. 让 `brun.yaml` 和时间过滤解析错误显式报错。
7. 再评估是否需要抽出 `internal/runner`。只有当调度、daemon 或 submit/start/cancel 开始实现时，才把运行编排从 `internal/cli/run.go` 迁入新的 runner 层。

## 任务调度与依赖运行

### 背景

当前 `brun run` 的模型是立即执行：创建 run 目录、保存命令/环境/脚本快照、写入 running 记录，然后启动命令。后续如果要支持“提交一个任务，指定时间运行”或“某个任务完成后运行”，需要在现有 run 记录之上增加一个轻量调度层。

目标不是把 brun 做成完整 workflow 引擎，而是提供适合日常生信开发的任务队列能力：

- 提交后暂不运行，到指定时间自动启动。
- 等某个 run 完成或成功后启动下游任务。
- Web Dashboard 能看到待运行任务。
- 计划变化时可以手动立即启动或取消。

### 推荐命令形态

```bash
# 指定时间运行
brun submit --at "2026-05-23 02:00" -- bash 04.sh

# 相对时间运行
brun submit --after 2h -- bash 04.sh

# 上游 run 结束后运行，不区分成功失败
brun submit --after-run 20260522-153012-a8f3c2 -- bash 05.sh

# 上游 run 成功后运行
brun submit --after-success 20260522-153012-a8f3c2 -- bash 05.sh

# 手动立即启动已提交但未运行的任务
brun start 20260523-020000-abc123

# 取消未运行任务
brun cancel 20260523-020000-abc123

# 修改计划时间
brun reschedule 20260523-020000-abc123 --at "2026-05-23 04:00"
```

### 状态模型

第一版建议保持简单：

```text
scheduled -> running -> success / failed
scheduled -> canceled
```

后续支持依赖后，可以扩展：

```text
pending -> running -> success / failed
pending -> canceled
```

其中：

- `scheduled`：等待指定时间。
- `pending`：等待依赖满足，或者等待时间和依赖同时满足。
- `running`：已经被 scheduler、Web 或 CLI 手动启动。
- `canceled`：用户取消，不能再被自动调度。

手动启动时不要删除原计划信息。建议记录：

```text
started_reason = manual | scheduler | rerun
```

这样后续审计时能看出任务原本是计划任务，但被提前启动了。

### 前端能力

Web Dashboard 应该把未运行任务作为 run 列表的一部分展示，而不是做单独页面。

列表页建议展示：

- 状态：`scheduled` / `pending`
- 计划时间：`scheduled_at`
- 距离启动时间：例如 `in 3h`
- 依赖条件：例如 `after_success: <run_id>`

详情页建议展示：

- 命令、CWD、project、tags、note
- 脚本快照
- 计划时间
- 依赖任务和依赖状态
- 启动原因

详情页操作：

- `Run now`：立即启动。
- `Cancel`：取消未运行任务。
- `Edit time`：修改计划时间。
- `Skip wait`：忽略时间或依赖条件直接启动。

### 调度器

普通 CLI 进程提交任务后会退出，所以自动启动必须依赖常驻进程。

可选方案：

1. `brun daemon`
   - 专门负责扫描并启动 due tasks。
   - 语义清楚，适合 systemd user service。

2. `brun web --scheduler`
   - Web Dashboard 启动时顺带运行 scheduler。
   - 对单机使用最方便。
   - 需要避免多个 Web 实例重复调度。

3. systemd user service
   - 后续可以生成 service 模板。
   - 不建议第一版强依赖 systemd。

第一版建议实现：

```bash
brun daemon
brun web --scheduler
```

其中 `--scheduler` 默认可以先关闭，避免用户无意启动多个调度器。

### 数据库变更

`runs` 表可新增字段：

```sql
scheduled_at TEXT;
queued_at TEXT;
started_reason TEXT;
```

依赖关系建议新增表：

```sql
CREATE TABLE run_dependencies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  upstream_run_id TEXT NOT NULL,
  condition TEXT NOT NULL,
  created_at TEXT NOT NULL
);
```

`condition` 可选：

```text
after_run
after_success
after_failure
```

### 并发与重复启动

必须通过数据库原子 claim 防止重复启动。scheduler、Web、CLI 都可能同时尝试启动同一个任务。

启动前做条件更新：

```sql
UPDATE runs
SET status = 'running', started_reason = ?
WHERE id = ?
  AND status IN ('scheduled', 'pending');
```

只有影响行数为 1 的调用方可以真正启动命令。影响行数为 0 表示任务已经被别人启动、取消或完成。

### 实现拆分

当前 `executeRun` 同时负责：

- 创建 run id 和 run 目录
- 保存 command/env/script snapshot
- 写入 running 记录
- 执行命令
- fs-diff
- 更新状态和资源

调度能力需要拆成两个阶段：

1. 创建任务记录
   - 分配 run id
   - 创建 run dir
   - 保存 command/env/script snapshot
   - 写入 `scheduled` 或 `pending`

2. 启动已有任务
   - 原子 claim
   - 执行命令
   - fs-diff
   - 更新 status/resources/artifacts

这个拆分是本功能的主要工程工作。

### 分阶段计划

#### Phase 1: 定时提交 + 手动启动/取消

范围：

- DB migration：`scheduled_at`, `queued_at`, `started_reason`
- Store 方法：创建 scheduled run、查询 due runs、claim、cancel、start
- CLI：`submit --at`, `submit --after`, `start`, `cancel`
- 抽出“启动已有 run”的执行路径
- `brun daemon` 扫描并启动到期任务
- Web 列表展示 `scheduled`
- Web 详情页支持 `Run now` 和 `Cancel`
- 测试 store/CLI/scheduler 核心路径

预估代码量：约 500-800 行净新增/修改。

#### Phase 2: 任务依赖

范围：

- 新增 `run_dependencies` 表
- CLI：`submit --after-run`, `submit --after-success`, `submit --after-failure`
- scheduler 检查依赖是否满足
- Web 展示依赖来源和依赖状态
- 测试成功、失败、未完成、取消等路径

预估代码量：约 300-600 行净新增/修改。

#### Phase 3: 编辑计划与更完整的 Web 操作

范围：

- CLI：`reschedule`
- Web：修改计划时间、跳过等待、显示启动原因
- 可选：Web 表单提交新任务
- 可选：生成 systemd user service 模板

预估代码量：约 300-700 行净新增/修改。

### 风险与注意事项

- 不要让 `submit` 直接执行命令；它只创建待运行任务。
- 手动启动和 scheduler 启动必须共用同一条启动路径。
- 需要保证同一个 scheduled run 不会被重复启动。
- run 目录、日志路径、脚本快照必须在提交时就确定，启动时复用。
- `brun web --scheduler` 多实例启动时要依赖数据库 claim 兜底。
- 调度器崩溃后重启，应能继续扫描未运行任务。
- 时间解析要明确时区，建议内部统一 UTC，输入按本地时区解释并在 UI 显示本地时间。

### 非目标

第一版不做：

- DAG workflow 编辑器。
- 多节点分布式调度。
- 资源配额和队列优先级。
- cron 表达式。
- 自动重试策略。

这些可以在轻量调度稳定后再考虑。
