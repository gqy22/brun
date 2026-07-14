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

### 当前阶段

短期不急着拆 `internal/store.go`、`internal/capture.go`、`internal/git.go` 和 `internal/logger.go`。这些文件职责单一、体量较小，继续拆成子包会带来较多 import churn，但收益有限。

当前架构已经够用一段时间。下一轮优先级不应继续做结构拆分，也不应急着进入调度系统；当前更值得做的是命令体验规范化、agent native 输出稳定化，以及 `docs/fallbacks.md` 中剩余行为的显式化。

运行诊断已经落地：`brun run` 会把推断行为和关键写入失败记录到 run 目录下的 `diagnostics.jsonl`，并在前台运行结束时输出 warning 摘要。诊断摘要已经写入 SQLite，`brun list`、`brun show` 和 Web 列表可以直接展示 warning/error 状态；`brun diag` 用于查看完整诊断事件。

SQLite 当前定位为快速索引层，run 目录中的 `command.sh`、`stdout.o`、`stderr.er`、`metadata.yaml` 和 `diagnostics.jsonl` 是主审计载体。默认 `BRUN_SQLITE_SYNC=off` 用于避免短任务被 SQLite 同步拖慢；需要更强写入一致性时可以设置 `BRUN_SQLITE_SYNC=normal` 或 `BRUN_SQLITE_SYNC=full`。如果索引缺失或损坏，使用 `brun repair --write` 从 run 目录重建缺失的 run 记录。

当前命令体系已经形成 5 组稳定入口：

1. 运行入口：`brun run -- <command>`。
2. 控制入口：`brun stop <run_id>`（终止运行中任务）。
3. 查询入口：`brun list`、`brun show`、`brun logs`、`brun outputs`、`brun script`、`brun diag`。
4. 标注和复用：`brun tag`、`brun note`、`brun rerun`。
5. 维护入口：`brun repair`、`brun clean`。
6. 工具界面和模板：`brun web`、`brun init`。

已完成：

1. 命令体验规范化：已支持 `brun -- <command>` 作为 `brun run -- <command>` 的快捷入口；只运行 `brun` 继续显示帮助，不默认执行任何动作；缺少 `--` 分隔符的写法会返回 `missing_command_separator`，并且会作为一条 `failed` run 落库（写入 `diagnostics.jsonl`、SQLite、`metadata.yaml`），而不是只输出错误就退出。
2. JSON 和错误契约补齐：`brun list/show/outputs/diag/clean` 均支持 JSON；常见可恢复错误输出稳定 `Code` 和 `Hint`。
3. `brun clean` 已从占位命令改为可执行维护命令：必须提供 `--older-than`，默认只预览，只有 `--write` 才删除匹配 run 记录和 run 目录；支持 `--json`、`--keep-failed`、`--keep-tag`。

近期建议优先级：

1. Git 采集状态显式化（hostname/username 已显式化完成；`hostname_status`/`username_status` 取值 `ok|unavailable`，与 `conda_status`/`resource_status` 同模式）。
2. 域函数从 `internal/cli/run.go` 移入 `internal/capture.go`（Step A，纯搬家，零行为变更；为后续拆 `internal/runner` 铺路）。
3. 调度 Phase 1 启动时拆 A/B、建 `internal/runner`（Step B，需先拍板"CWD 何时确定 / env 何时冻结"等语义问题；不做抢跑）。

已完成：是否需要抽出 `internal/runner` 已完成评估，结论分两步走。
当前 `executeRun`（`internal/cli/run.go:122-411`）是一个 290 行单体函数，混合了"准备 run 记录"和"真正执行"两段语义，调度 Phase 1 必须在"打开 DB、写 running 记录"那一行切开成 A/B。直接现在切 A/B 会强行把"CWD 何时检测 / env 何时冻结"等尚未拍板的产品问题提前暴露，属于提前抽象。因此拆 `internal/runner`（Step B）暂不做，与调度 Phase 1 同步启动。但当前可以零成本做 Step A：`detectCWD` / `findScriptArg` / `isTextFile` / `hostname` / `username` 五个域函数被卡在 `cli` 包里，阻碍 `internal/` 单测和未来 `runner` 复用，把它们移入 `internal/capture.go` 是纯搬家、不改任何行为。

已完成：Web 启动语义已收紧，显式 `--port` 被占用时会直接失败；未显式指定端口时才保留自动寻找后续端口。Web processes/logs API 已返回 `process_source`、`activity_sampled`、`last_log_status` 和 logs `status`，避免把降级状态伪装成空值。

已完成：Conda 状态已进入 run 审计链路。`brun run` 会记录 `conda_status=ok|partial|not_detected`、`conda_env`、`conda_prefix` 和 `python_version`，并写入 SQLite、`metadata.yaml`、`show --json` 和 Web detail。

已完成：资源采样能力已显式化。`brun run` 会记录 `resource_supported` 和 `resource_status=ok|unavailable|unsupported`，并写入 SQLite、`metadata.yaml`、`show --json` 和 Web detail，避免把“不支持采样”显示成真实 0。

已完成：运行控制入口已统一为 `brun stop <run_id>`。终止采用两阶段：先发 SIGTERM，等待最多 3 秒宽限期，进程仍未退出再发 SIGKILL；支持 `--force` 跳过宽限期，支持 `--latest` 选择器，作用于整个进程组（含子进程）。终止路径已收敛到 `cmd.StopRun()`，CLI `brun stop` 和 Web kill 共享同一条代码路径，Web 侧改为等待进程真正退出再返回。终止后状态自动置为 `failed`，并保留最近一次资源采样。

已完成：hostname/username 采集状态已显式化。`brun run` 启动时尝试 `os.Hostname()` 和 `USER` 环境变量：成功写入 `hostname_status=ok`/`username_status=ok`，失败或为空时写入 `unavailable`。字段已并入 SQLite schema（version 6 迁移加 `hostname_status` / `username_status` 两列）、`metadata.yaml`、`brun show --json` / `brun list --json` 输出和 Web detail。`brun repair` 从 `metadata.yaml` 重建索引时也会带回这两个状态。

已完成：展示层状态 `success_with_warnings` / `failed_with_warnings` / `cancelled_with_warnings` 已落地。`Run.DisplayStatus()` 在 `DiagWarningCount>0` 且基础状态为 `success/failed/cancelled` 时分别返回三种 `_with_warnings` 变体，其他情况原样返回 `Status`。`brun list/show --json`、Web `/api/runs` 列表与详情都透出 `display_status` 字段；Web 状态筛选下拉新增 `failed_with_warnings` 和 `cancelled_with_warnings` 选项，行色与徽标用 `*_with_warnings` CSS 变体高亮。`status` 字段保持原值不变，agent 工具按 `status` 过滤不被打乱。

已完成：schema 升至 v7 并加入 `hostname_status` / `username_status` 的迁移回填。v5 → v7 升级时，对已存在的 `runs` 行执行：`hostname`/`username` 非空则回填 `ok`，否则回填 `unavailable`；`WHERE *_status IS NULL` 限定保证幂等，老库升级一次即可，不再二次改写。

## 中长期：任务调度与依赖运行

### 背景

当前 `brun run` 的模型是立即执行：创建 run 目录、保存命令/环境/脚本快照、写入 running 记录，然后启动命令。后续如果要支持“提交一个任务，指定时间运行”或“某个任务完成后运行”，需要在现有 run 记录之上增加一个轻量调度层。

目标不是把 brun 做成完整 workflow 引擎，而是提供适合日常生信开发的任务队列能力：

- 提交后暂不运行，到指定时间自动启动。
- 等某个 run 完成或成功后启动下游任务。
- Web Dashboard 能看到待运行任务。
- 计划变化时可以手动立即启动或取消。

### 技术选型：开源库评估

动手前评估了 Go 生态的现成方案，确认核心调度逻辑没有可直接复用的库、需要自写，定时部分用标准库即可。结论与依据记录如下，避免后续重复调研。

评估约束（由 brun 定位决定）：

- 纯 Go、单二进制，**不依赖外部服务**（Redis / Postgres / 独立 server）。
- 调度状态需**持久化**到现有 SQLite，进程重启可恢复。
- DAG 语义是定制的（两种相关性、失败传播），通用引擎难以匹配。
- 必须能**嵌入** brun web，而不是一个独立产品。

评估结果：

| 库 | 外部依赖 | 是否适配 brun |
|---|---|---|
| `robfig/cron`、`go-co-op/gocron`、`reugn/go-quartz` | 无 | 仅做定时 / cron 表达式；**内存态**，重启即丢，持久化仍要自写；不解决任务依赖 |
| `hibiken/asynq` | 🔴 Redis | 任务链 + 定时 + 持久化最全，但依赖 Redis，违背单机定位 |
| `riverqueue/river` | 🔴 Postgres | 依赖 Postgres |
| `dagu-dev/dagu` | pgx / redis / grpc | 完整 workflow **产品**（非库），不可嵌入 |
| `looplab/fsm` | 无 | 仅状态机，解决不了调度 |

分功能结论：

1. **定时（一次性）**：标准库 `time.AfterFunc` 足够，不引库。
2. **定时（周期，可选）**：若后续做 cron 表达式，可借 `robfig/cron` 解析表达式与计算下次触发（约省 50 行），但 `scheduled_at` 仍需自存 SQLite 并在重启后重新注册。
3. **DAG 依赖调度**：没有符合条件的库，**自写**。核心逻辑清晰（环检测 + 前置满足判断 + 失败传播），核心版约 850 行，符合"调度逻辑本身重要、值得做扎实"的定位。
4. **整体**：不引入外部依赖，核心调度自写。这是约束筛选的结果，不是盲目造轮子。

备选（第一版不采用）：若部署环境本就有 Redis，`hibiken/asynq` 能覆盖队列 + 依赖链 + 定时 + 持久化 + 监控，可省大部分自写代码；但要求用户额外维护 Redis，与 brun"开箱即用、单机单二进制"的定位冲突，暂不采用。后续若用户场景普遍已有 Redis，可重新评估。

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

`condition` 取值（默认 `after_run`）：

```text
after_run       上游进入任意终态（成功/失败/取消）即触发，不区分成败
after_success   上游必须成功；上游失败/取消则本任务不执行
after_failure   上游必须失败才触发
```

默认使用 `after_run`（弱相关）的理由：多数下游任务并不真正依赖上游产物，上游失败不应拖累无关的下游任务。仅当下游确实消费上游产物时，才显式声明 `after_success`（强相关），上游失败会让下游不执行。该决策的取舍是"宁可继续跑无关任务，也不因一个失败连锁阻断整批"。

"不执行"的落库状态：第一版可复用 `canceled`；若需在审计上区分"被依赖阻断"与"用户主动取消"，后续引入 `blocked` / `skipped` 状态。

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
- **全局并发上限**：不限制同时运行的任务数，执行顺序完全由用户显式声明的依赖关系决定（手动控制优先于系统级资源调度）。如果后续发现用户依赖设漏导致机器过载，再考虑加可选的全局上限作为兜底。
- cron 表达式。
- 自动重试策略。

这些可以在轻量调度稳定后再考虑。
