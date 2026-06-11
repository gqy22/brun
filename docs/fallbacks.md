# Fallback Audit

本文档记录当前项目中存在的 fallback、默认值、静默忽略和降级实现。目标是让这些行为显式化，后续逐步删除不必要的 fallback，或改成明确的错误、配置项和用户可见提示。

## 分类原则

- `default`：未指定输入时使用默认值。
- `infer`：根据上下文推断值。
- `silent-ignore`：错误或缺失被静默忽略。
- `retry`：失败后自动重试。
- `degrade`：平台或环境不支持时返回空实现。
- `classification-default`：无法识别时归入默认类别。
- `resolved`：曾经存在的 fallback 或隐式行为，已经清理；保留记录用于防止回归。

## 当前清单

### 1. BRUN_HOME 默认目录

- 类型：`default`
- 位置：`internal/git.go:20`
- 当前行为：如果未设置 `BRUN_HOME`，使用 `$HOME/.brun`。不会探测、迁移或兼容旧的 `$HOME/.bio-runner`。
- 影响：默认路径更短、更符合工具命名；已有本地旧数据需要之后单独迁移，当前版本不会自动搬运。
- 建议：保留新默认值。后续如需迁移旧目录，应提供显式迁移命令，而不是在启动时自动兼容多个目录。

### 2. run_id 短 ID 路径

- 类型：`default`
- 位置：`internal/git.go:27`
- 当前行为：`runID` 长度小于 8 时直接放到 `runs/<runID>`，否则按日期分层。
- 影响：对异常 ID 做了隐式兼容，可能掩盖调用方传入非法 run id。
- 建议：后续可改为显式校验 run id 格式；异常 ID 返回错误。

### 3. Project 名推断

- 类型：`visible-infer`
- 位置：`internal/git.go:48`
- 当前行为：优先级为 `--project` > `brun.yaml project` > 当前目录名。
- 影响：当前目录名作为项目名可能导致记录归错项目，尤其是在临时目录、软链接目录或脚本自动运行场景。
- 当前治理：run 记录已写入 `project_source=explicit|config|inferred`，`brun show` 和 Web API 会展示来源；推断时也会写入 info 级诊断。
- 建议：短期保留目录名推断，但继续在 UI/CLI 中明确展示来源。

### 4. brun.yaml 解析失败后使用默认配置

- 类型：`resolved`
- 位置：`internal/cli/run.go`
- 当前行为：读取到 `brun.yaml` 后，如果 `ParseConfig` 失败，`brun run` 会直接报错，命令不会启动。
- 影响：错误配置不会再被默认配置掩盖。
- 建议：保持严格行为，不增加继续运行的兼容开关。

### 5. Git 信息采集失败被忽略

- 类型：`silent-ignore`
- 位置：`internal/git.go:72`
- 当前行为：不是 Git repo 或 git 命令失败时，repo/branch/commit 留空。
- 影响：对非 Git 目录友好，但 git 命令异常、权限问题、坏 repo 也会被吞掉。
- 建议：区分“不是 Git repo”和“Git repo 但采集失败”。前者可以允许，后者应记录 warning 或报错。

### 6. 命令工作目录自动推断

- 类型：`visible-infer`
- 位置：`internal/cli/run.go:464`
- 当前行为：如果首个参数是已存在脚本文件，使用脚本所在目录；否则回退当前目录。
- 影响：行为不透明。同一个命令在脚本路径存在或不存在时 CWD 不同，可能造成输出文件位置变化。
- 当前治理：run 记录已写入 `cwd_source=explicit|inferred`，`brun show` 和 Web API 会展示来源；推断时也会写入 info 级诊断。
- 建议：短期保留自动推断，但帮助信息和详情页必须继续提示来源。

### 7. 脚本快照自动查找失败后跳过

- 类型：`diagnosed-default`
- 位置：`internal/cli/run.go`
- 当前行为：尝试在参数中查找脚本文件；找不到可读脚本时不保存快照，并写入 info 级诊断。找到脚本但保存失败时写入 warning。
- 影响：直接运行二进制或 `sh -c` 时不会产生噪音，但用户仍需要打开诊断文件才能确认没有脚本快照。
- 建议：提供 `--script` 显式指定快照文件；如果命令形态明显包含脚本但无法读取，再提升为 warning。

### 8. 保存 command/env/script 失败被忽略

- 类型：`partial-resolved`
- 位置：`internal/cli/run.go`
- 当前行为：`command.sh` 或 `env.txt` 写入失败时直接报错，命令不会启动；脚本快照失败仍写入 warning 并继续运行。
- 影响：关键审计文件不再缺失后继续执行；脚本快照仍是辅助审计，允许降级。
- 建议：保留当前分层：command/env 强约束，script snapshot 诊断化。

### 9. 文件系统 before/after 快照失败被忽略

- 类型：`diagnosed-degrade`
- 位置：`internal/cli/run.go`
- 当前行为：before 或 after `SnapshotDir` 失败时写入 warning；只有两次快照都成功时才执行 fs-diff 和 artifact 记录。
- 影响：避免了误判 artifact，但快照失败仍不会阻断 run。
- 建议：默认开启 fs-diff 时，快照失败可以继续允许命令完成，但应在 run metadata 中记录 `fs_diff_status`，方便 list/web 直接展示。

### 10. 遍历目录时单文件错误被忽略

- 类型：`silent-ignore`
- 位置：`internal/capture.go:36`
- 当前行为：`filepath.Walk` 遇到单个文件错误时 `return nil`，继续遍历。
- 影响：权限错误、坏链接、瞬时删除都不会进入结果，也没有提示。
- 建议：收集 skipped files，返回附带 warning；或在严格模式下报错。

### 11. Artifact 写入失败被忽略

- 类型：`diagnosed-degrade`
- 位置：`internal/cli/run.go`
- 当前行为：`store.CreateArtifact` 返回错误时写入 warning，并在 run 结束摘要中提示；run 状态仍按命令退出码更新。
- 影响：用户能通过 `brun list` 的 `DIAG`、`brun show`、`brun diag` 和 Web 详情知道 artifact 入库失败；诊断摘要已入库，但 run 状态仍是 `success` 或 `failed`。
- 建议：后续如需要更强语义，可增加展示层状态 `success_with_warnings`，但不要覆盖真实命令退出状态。

### 12. tag/note 写入失败被忽略

- 类型：`diagnosed-degrade`
- 位置：`internal/cli/run.go`
- 当前行为：`store.AddTag`、`store.AddNote` 返回错误时写入 warning，并在 run 结束摘要中提示；run 状态仍按命令退出码更新。
- 影响：用户能知道标签或备注丢失，但写入失败不会阻断运行结果。
- 建议：删除该 degrade。用户显式传入的标签和备注写入失败应报错，或标记 `success_with_warnings`。

### 13. 资源数据更新失败被忽略

- 类型：`diagnosed-degrade`
- 位置：`internal/cli/run.go`
- 当前行为：`UpdateRunResources` 返回错误时写入 warning，并在 run 结束摘要中提示。
- 影响：资源数据可能丢失，但命令结果不会被资源写入失败覆盖。
- 建议：保留运行状态更新优先级，同时在 run metadata 或诊断摘要中显示资源写入状态。

### 14. metadata.yaml 写入失败被忽略

- 类型：`diagnosed-degrade`
- 位置：`internal/cli/run.go`
- 当前行为：`os.WriteFile(metadata.yaml)` 返回错误时写入 warning，并在 run 结束摘要中提示。
- 影响：run 目录中的离线元数据可能缺失，但 SQLite run 记录仍会更新；`repair` 无法从缺失的 metadata 重建这条 run。
- 当前治理：metadata 写入失败会进入 run 级诊断摘要，`brun list` 和 Web 列表不需要扫描 `diagnostics.jsonl` 也能看到警告。
- 建议：继续保留 SQLite 更新优先级。

### 15. 失败摘要读取 stderr 失败被忽略

- 类型：`silent-ignore`
- 位置：`internal/cli/run.go:313`
- 当前行为：读取 stderr 失败时不显示摘要。
- 影响：低风险，只影响失败后的辅助展示。
- 建议：可以保留，但最好打印“无法读取 stderr 摘要”的提示。

### 16. allow-exit 把失败改成功

- 类型：`default`
- 位置：`internal/cli/run.go:288`
- 当前行为：如果 exit code 在 `--allow-exit` 中，最终状态从 `failed` 改成 `success`。
- 影响：这是显式用户配置，不属于坏 fallback，但会改变真实失败语义。
- 建议：保留，但 metadata 中应同时记录真实 exit code 和 status override reason。

### 17. 后台 detached run 预生成 runID

- 类型：`resolved`
- 位置：`internal/cli/run.go:376`
- 当前行为：后台父进程预生成 runID，并通过隐藏 `--run-id` 传给子进程；父进程打印的 runID、run 目录和数据库记录一致。
- 影响：原先的语义不一致已经修复。
- 建议：保留回归测试；后续不要再引入父子进程各自生成 runID 的路径。

### 18. Web addr 和 port 默认值

- 类型：`default`
- 位置：`internal/cli/manage.go:207`
- 当前行为：空 addr 使用 `0.0.0.0`，空/零 port 使用 `9213`。
- 影响：`0.0.0.0` 默认暴露到局域网，可能不是最安全的默认。
- 建议：默认监听 `127.0.0.1`，局域网访问必须显式 `--addr 0.0.0.0`。

### 19. Web 端口自动递增

- 类型：`partial-resolved`
- 位置：`cmd/web.go:63`
- 当前行为：未显式指定 `--port` 时，如果默认端口被占用，会自动尝试后续最多 20 个端口；显式指定 `--port` 时，端口不可用会直接报错并输出 `web_listen_failed`。
- 影响：脚本化场景不会再出现指定 `--port 9090` 但实际监听 9091+ 的行为漂移；默认交互场景仍能自动避开占用端口。
- 建议：保留当前分层。后续如果需要完全可预测，也可以增加 `--no-port-fallback` 或改为默认也不递增。

### 20. Conda 环境信息缺失时静默降级

- 类型：`partial-resolved`
- 位置：`cmd/root.go`, `internal/cli/run.go`, `internal/store.go`
- 当前行为：`brun run` 会采集当前 Conda 状态并写入 SQLite、`metadata.yaml`、`show --json` 和 Web detail。字段包括 `conda_status=ok|partial|not_detected`、`conda_env`、`conda_prefix`、`python_version`。
- 影响：run 审计中不再只能看到空值；智能体可以区分没有激活 Conda、检测到部分信息和完整检测成功。
- 建议：保持 run 审计优先。`brun init` 仍只使用简短字符串注释，不为低频模板命令增加复杂状态。

### 21. 非 Linux 资源采样空实现

- 类型：`resolved`
- 位置：`cmd/resource_other.go`, `cmd/run.go`, `internal/store.go`
- 当前行为：资源采样结果会同时写入 `resource_supported` 和 `resource_status=ok|unavailable|unsupported`。Linux 返回 `resource_supported=true`；非 Linux 返回 `resource_supported=false` 和 `resource_status=unsupported`。
- 影响：Web/API 和智能体可以区分“不支持采样”“支持但未采到数据”和“正常采到资源数据”，不会把不支持误判成真实 0。
- 建议：保持能力字段。后续如需更细，可以增加采样失败原因。

### 22. Artifact 类型默认 output

- 类型：`classification-default`
- 位置：`internal/capture.go:87`
- 当前行为：无法识别类型时归为 `output`。
- 影响：未知文件会被误标成输出。
- 建议：增加 `unknown` 类型；或者只对 capture 配置命中的路径分类为 output。

### 23. 数据库 busy 自动重试

- 类型：`retry`
- 位置：`internal/store.go:125`
- 当前行为：遇到 `SQLITE_BUSY` 或 `database is locked` 指数退避重试。
- 影响：这是合理的并发保护，不建议删除。
- 建议：保留，但参数应集中配置，失败时错误信息保留重试次数和总耗时。

### 24. 数据库 migration 重复列忽略

- 类型：`silent-ignore`
- 位置：`internal/store.go:116`
- 当前行为：`ALTER TABLE ADD COLUMN` 出现 duplicate column 时忽略。
- 影响：简化迁移重复执行，但依赖错误字符串匹配。
- 建议：短期保留。长期改成版本化 migration 表，避免靠错误字符串判断。

### 25. 读取 note 不存在时返回空字符串

- 类型：`default`
- 位置：`internal/store.go:335`
- 当前行为：没有 note 时返回 `""` 且无错误。
- 影响：合理的领域默认值。
- 建议：保留。

### 26. Script snapshot 过大或二进制时跳过

- 类型：`silent-ignore`
- 位置：`cmd/query.go:56`
- 当前行为：读取脚本快照时，大于 2MB 或包含 NULL 字节则继续找下一个，最后可能报“未找到”。
- 影响：用户不知道是没有快照，还是快照被大小/二进制规则过滤。
- 建议：返回具体原因，例如 `snapshot too large` 或 `snapshot is binary`。

### 27. 时间过滤严格解析

- 类型：`resolved`
- 位置：`internal/cli/query.go:367`
- 当前行为：`parseTimeFilter` 只接受 `YYYY-MM-DD`、RFC3339、`today`、`Nh`、`Nd`、`Nw`。非法格式会直接报错。
- 影响：用户和智能体不会再把错误时间条件误解为空结果。
- 建议：保持严格解析。

### 28. hostname 获取失败返回空

- 类型：`resolved`
- 位置：`internal/cli/run.go:591`
- 当前行为：`os.Hostname()` 错误或返回空字符串时，写入 `hostname_status=unavailable` 和空 `hostname`；正常路径写入 `hostname_status=ok`。
- 影响：可以区分"未采集到"和"采集到但字段为空"。
- 建议：保持显式状态；不需要再加 warning 诊断（采集失败本身不影响 run 主体）。

### 29. username 直接读取 USER

- 类型：`resolved`
- 位置：`internal/cli/run.go:596`
- 当前行为：`USER` 环境变量存在时写入 `username_status=ok`，否则写入 `username_status=unavailable`。
- 影响：在非交互环境或 Windows 上不再是隐式空字符串，状态显式可见。
- 建议：保持显式状态；如果后续要支持 `os/user` 回退，需要在 metadata 中区分来源（与 git/git_status 同样的模式）。

### 30. README 和代码端口默认值曾不一致

- 类型：`documentation-fallback`
- 位置：`README.md:119`, `internal/cli/manage.go:192`
- 当前行为：已修正为 README、代码和 help 都使用 `9213`。
- 影响：历史文档曾写成 `9313`，用户按旧文档访问会失败。
- 建议：保持 README、CLI help 和代码默认值同步；如果后续调整默认端口，三处必须一起更新。

### 31. 只读 list 在数据库不存在时退回可写建库

- 类型：`degrade`
- 位置：`internal/cli/root.go:126`, `internal/store.go:72`
- 当前行为：`brun list` 正常使用只读 SQLite 连接；如果 `db.sqlite` 不存在，则退回 `NewStore` 创建数据库并执行 migration。
- 影响：首次运行 `brun list` 仍会产生写操作，和“只读查询命令”的直觉不完全一致。
- 建议：短期可接受，用于保持历史行为。后续可以改成显示“未找到运行记录”，同时只在写命令或显式 `brun init-db` 时创建数据库。

### 32. 进程树采集失败后退回进程组扫描

- 类型：`partial-resolved`
- 位置：`cmd/web.go:375`, `cmd/resource_linux.go:158`
- 当前行为：Web processes API 先按 `/proc/<pid>/task/<pid>/children` 递归采集进程树；如果没有结果，则退回 `ListProcessGroup(pid)`；响应中返回 `process_source=tree|group|empty`。
- 影响：退回进程组扫描后可能丢失 depth/role 层级语义，也可能漏掉跨进程组但仍是子孙的进程。
- 建议：保留响应字段。后续如需更强诊断，可以把采样失败原因也写入 run 级诊断。

### 33. 进程 activity 采样失败时沿用第一帧

- 类型：`partial-resolved`
- 位置：`cmd/resource_linux.go:158`
- 当前行为：`ListProcessTreeWithActivity` 先采第一帧，短暂等待后采第二帧；如果第二帧为空，则返回第一帧并按状态推断 active。Web processes API 返回 `activity_sampled=true|false`。
- 影响：`active_count` 和 `cpu_delta_ms` 可能偏保守，用户无法区分“进程确实不活跃”和“采样窗口内进程消失/采样失败”。
- 建议：保留 `activity_sampled`。后续可增加 `sample_interval_ms`，让 API 消费方知道采样窗口。

### 34. Last log update 缺失时返回空值

- 类型：`resolved`
- 位置：`cmd/web.go:727`
- 当前行为：如果 stdout/stderr 都不存在或无法 stat，`last_log_update` 和 `last_log_update_ago` 仍为空，同时返回 `last_log_status=missing|unreadable|ok`。普通 logs API 也返回 `status=missing|unreadable|ok`。
- 影响：UI 和智能体可以区分“没有日志文件”“日志不可读”和“日志可读但无新增内容”。
- 建议：保持显式状态字段。

### 35. 进程角色无法识别时归为 worker

- 类型：`classification-default`
- 位置：`cmd/resource_linux.go:417`
- 当前行为：进程角色按 root、runner、shell 判断；未命中时默认 `worker`。
- 影响：未知进程会被展示成 worker，可能把 tee、tail、监控辅助进程和真正计算进程混在一起。
- 建议：增加 `process` 或 `unknown` 角色，把默认分类从 worker 调整为 unknown；worker 只用于更明确的计算进程规则。

### 36. SQLite 默认低同步作为快速索引

- 类型：`explicit-performance-default`
- 位置：`internal/store.go`
- 当前行为：SQLite 默认使用 `BRUN_SQLITE_SYNC=off`，把数据库定位为快速索引层；run 目录中的 `metadata.yaml`、日志和诊断文件是主审计载体。用户可以设置 `BRUN_SQLITE_SYNC=normal` 或 `BRUN_SQLITE_SYNC=full` 提升写入一致性。
- 影响：极端情况下，例如机器断电或文件系统崩溃，最近一次 SQLite 写入可能丢失或损坏，但 run 目录仍保留审计文件。
- 建议：保留该默认值以保证短任务体验。当前已提供 `brun repair --write` 从 run 目录重建缺失 run 索引；后续如需恢复 artifacts/tags/notes，需要扩展 metadata 或增加独立审计文件。

### 37. latest 伪 run id

- 类型：`resolved`
- 位置：`internal/cli/query.go`, `internal/cli/manage.go`, `cmd/query.go`
- 当前行为：`latest` 不再作为位置参数伪装成 run id；查询和管理命令统一使用显式 `--latest`。例如 `brun show --latest`、`brun logs --latest`、`brun tag --latest TAG...`。
- 影响：旧的 `brun show latest` 会按普通 run id 查询并失败，不再触发隐藏选择逻辑。CLI 语义更清楚，也避免把特殊词混进 run id 空间。
- 建议：保留这种显式选择器模式。新增命令也应使用短命令名加显式 flag，例如 `brun diag --latest`，不要新增长命令或兼容别名。

### 38. 查询输出只能解析人类文本

- 类型：`resolved`
- 位置：`internal/cli/query.go`
- 当前行为：`brun list`、`brun show`、`brun outputs` 已支持 `--json`；`brun diag --json` 继续提供完整诊断事件。
- 影响：智能体和脚本不需要解析表格或彩色文本，可以直接消费 snake_case JSON 字段。
- 建议：新增查询类命令时默认同时提供人类输出和 `--json`。

### 39. 常见 CLI 错误缺少稳定 code/hint

- 类型：`partial-resolved`
- 位置：`internal/cli/errors.go`, `internal/cli/root.go`
- 当前行为：常见可恢复错误会输出 `Error`、`Code` 和 `Hint`，目前覆盖时间过滤、run 选择器、run 查找、配置解析、命令不存在、run 目录创建、command/env 审计文件写入失败。
- 影响：智能体可以根据稳定错误码决定修参数、查 run 列表、修配置、检查权限或终止流程。
- 建议：继续把数据库损坏、索引缺失、日志文件缺失、Web 端口占用等错误迁入同一格式。

### success_with_warnings 展示状态

- 类型：`resolved`
- 位置：`internal/store.go`（`Run.DisplayStatus()`）、`internal/cli/query.go`、`cmd/query.go`、`cmd/web.go`
- 当前行为：`Run.DisplayStatus()` 在 `Status=success` 且 `DiagWarningCount>0` 时返回 `success_with_warnings`，否则原样返回 `Status`。`brun list/show --json`、Web `/api/runs` 列表与详情都透出 `display_status` 字段。`status` 字段保持原值不变，agent 工具按 `status` 过滤不会被打乱。
- 影响：智能体和前端可以在不读取诊断详情的前提下，快速区分"真成功"和"成功但记录链路有 warning"。
- 建议：保持 `status` 与 `display_status` 双字段约定；如果未来引入 `failed_with_warnings` 或 `cancelled_with_warnings` 等同族语义，集中在 `DisplayStatus()` 一个函数里扩展。

## 建议清理顺序

1. 继续处理会造成行为漂移的 fallback：Web 默认监听地址是否继续保留局域网默认，或增加更明确的安全提示。
2. 把剩余高频失败迁入结构化错误：数据库损坏/缺失、日志文件不可读。
3. 再处理合理但需要显式化的 fallback：Git 信息采集、hostname/username 采集、HomeDir 默认值。
4. 最后评估是否需要展示层状态 `success_with_warnings`，用于表达命令成功但记录链路存在诊断警告。

## 下一轮优化方向

当前已经不需要继续做目录架构拆分。下一轮更值得做的是“Web 启动语义”和“剩余状态显式化”：

1. Web 默认监听地址重新评估：继续默认 `0.0.0.0` 需要在 help 和启动输出中足够明确；如改为 `127.0.0.1`，局域网访问必须显式 `--addr 0.0.0.0`。
2. 剩余错误结构化：数据库损坏/缺失提示 `repair`，日志不可读提示 run 状态和 run_dir。
3. Git 采集状态显式化（hostname/username 已显式化完成）。

## 保留候选

以下 fallback 当前看起来合理，但仍建议显式记录：

- `BRUN_HOME` 未设置时使用 `$HOME/.brun`。
- SQLite busy 自动重试。
- `brun list` 在数据库不存在时创建空库。
- note 不存在时返回空字符串。
- `--allow-exit` 显式覆盖状态。
- `--latest` 显式选择最新 run。
