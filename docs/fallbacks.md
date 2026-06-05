# Fallback Audit

本文档记录当前项目中存在的 fallback、默认值、静默忽略和降级实现。目标是让这些行为显式化，后续逐步删除不必要的 fallback，或改成明确的错误、配置项和用户可见提示。

## 分类原则

- `default`：未指定输入时使用默认值。
- `infer`：根据上下文推断值。
- `silent-ignore`：错误或缺失被静默忽略。
- `retry`：失败后自动重试。
- `degrade`：平台或环境不支持时返回空实现。
- `classification-default`：无法识别时归入默认类别。

## 当前清单

### 1. BRUN_HOME 默认目录

- 类型：`default`
- 位置：`internal/git.go:20`
- 当前行为：如果未设置 `BRUN_HOME`，使用 `$HOME/.bio-runner`。
- 影响：用户可能不知道数据写入位置；如果 `$HOME` 为空，路径也会变得不可靠。
- 建议：保留默认值可以接受，但启动时或 `brun info` 中应明确展示实际数据目录。也可以要求关键写操作先确认 `HomeDir()` 非空。

### 2. run_id 短 ID 路径

- 类型：`default`
- 位置：`internal/git.go:27`
- 当前行为：`runID` 长度小于 8 时直接放到 `runs/<runID>`，否则按日期分层。
- 影响：对异常 ID 做了隐式兼容，可能掩盖调用方传入非法 run id。
- 建议：后续可改为显式校验 run id 格式；异常 ID 返回错误。

### 3. Project 名推断

- 类型：`infer`
- 位置：`internal/git.go:48`
- 当前行为：优先级为 `--project` > `brun.yaml project` > 当前目录名。
- 影响：当前目录名作为项目名可能导致记录归错项目，尤其是在临时目录、软链接目录或脚本自动运行场景。
- 建议：可以保留 `--project` 和 `brun.yaml`；是否允许目录名 fallback 应变成显式配置，例如 `--infer-project` 或初始化时写入配置。

### 4. brun.yaml 解析失败后使用默认配置

- 类型：`diagnosed-degrade`
- 位置：`internal/cli/run.go`
- 当前行为：读取到 `brun.yaml` 后，如果 `ParseConfig` 失败，会写入 `diagnostics.jsonl` 并在 run 结束摘要中提示 warning，然后继续使用默认配置。
- 影响：用户已经能看到配置解析失败，但错误配置仍不会阻断运行，ignore/capture/project 可能不生效。
- 建议：下一步删除该 degrade。存在 `brun.yaml` 但解析失败时应直接报错，或提供显式 `--ignore-config-errors`。

### 5. Git 信息采集失败被忽略

- 类型：`silent-ignore`
- 位置：`internal/git.go:72`
- 当前行为：不是 Git repo 或 git 命令失败时，repo/branch/commit 留空。
- 影响：对非 Git 目录友好，但 git 命令异常、权限问题、坏 repo 也会被吞掉。
- 建议：区分“不是 Git repo”和“Git repo 但采集失败”。前者可以允许，后者应记录 warning 或报错。

### 6. 命令工作目录自动推断

- 类型：`infer`
- 位置：`internal/cli/run.go:464`
- 当前行为：如果首个参数是已存在脚本文件，使用脚本所在目录；否则回退当前目录。
- 影响：行为不透明。同一个命令在脚本路径存在或不存在时 CWD 不同，可能造成输出文件位置变化。
- 建议：后续优先使用显式 `--cwd`。自动推断可以删除，或只在 `brun run-script <path>` 这类专门命令中启用。

### 7. 脚本快照自动查找失败后跳过

- 类型：`diagnosed-default`
- 位置：`internal/cli/run.go`
- 当前行为：尝试在参数中查找脚本文件；找不到可读脚本时不保存快照，并写入 info 级诊断。找到脚本但保存失败时写入 warning。
- 影响：直接运行二进制或 `sh -c` 时不会产生噪音，但用户仍需要打开诊断文件才能确认没有脚本快照。
- 建议：提供 `--script` 显式指定快照文件；如果命令形态明显包含脚本但无法读取，再提升为 warning。

### 8. 保存 command/env/script 失败被忽略

- 类型：`diagnosed-degrade`
- 位置：`internal/cli/run.go`
- 当前行为：`SaveCommandFile`、`SaveEnvFile`、`SaveInputScript` 返回错误时写入 warning，并在 run 结束摘要中提示；run 仍继续执行。
- 影响：关键审计文件可能缺失，但用户已经能看到诊断提示。
- 建议：继续收紧。`command.sh` 和 `env.txt` 保存失败应阻断运行；脚本快照失败可以保持 warning。

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
- 影响：用户能知道 artifact 入库失败，但 list/web 仍可能看不到这些输出文件。
- 建议：如果 artifact 是核心能力，后续应在 run metadata 中记录 artifact 写入失败，或把 run 标为 `success_with_warnings`。

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
- 影响：run 目录中的离线元数据可能缺失，但 SQLite run 记录仍会更新。
- 建议：继续保留 SQLite 更新优先级；metadata 写入失败应在 list/web 中可见。

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

- 类型：`default`
- 位置：`internal/cli/run.go:376`
- 当前行为：后台父进程生成一个 runID 用于 stdout/stderr 路径；子进程真正执行时会再次生成自己的 runID。
- 影响：用户看到的 `[nohup] RunID` 可能不是数据库中的实际 run id，这是比 fallback 更严重的语义不一致。
- 建议：优先修复。父子进程应共享同一个 run id，或父进程不要打印 run id。

### 18. Web addr 和 port 默认值

- 类型：`default`
- 位置：`internal/cli/manage.go:207`
- 当前行为：空 addr 使用 `0.0.0.0`，空/零 port 使用 `9213`。
- 影响：`0.0.0.0` 默认暴露到局域网，可能不是最安全的默认。
- 建议：默认监听 `127.0.0.1`，局域网访问必须显式 `--addr 0.0.0.0`。

### 19. Web 端口自动递增

- 类型：`retry`
- 位置：`cmd/web.go:63`
- 当前行为：指定端口占用时自动尝试后续最多 20 个端口。
- 影响：用户指定 `--port 9090` 但实际可能监听 9091+，脚本化场景容易出错。
- 建议：如果用户显式传了 `--port`，端口被占用应报错；只有未显式指定端口时才可自动递增，并清晰提示。

### 20. Conda 环境信息缺失时静默降级

- 类型：`silent-ignore`
- 位置：`cmd/root.go:133`
- 当前行为：没有 `CONDA_DEFAULT_ENV` 返回空；Python 版本或 conda history 读取失败也静默跳过。
- 影响：脚本模板中的环境信息可能为空或不完整。
- 建议：对 init 模板生成来说可以接受，但应把空值渲染成明确文本，例如 `not detected`。

### 21. 非 Linux 资源采样空实现

- 类型：`degrade`
- 位置：`cmd/resource_other.go:1`
- 当前行为：非 Linux 平台资源采样返回 0，进程列表返回 nil。
- 影响：跨平台可运行，但 Web/API 中资源信息可能看起来像真实的 0。
- 建议：引入 capability 标记，例如 `resource_supported=false`，避免把“不支持”显示成“0”。

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

### 27. 时间过滤解析失败原样返回

- 类型：`silent-ignore`
- 位置：`internal/cli/query.go:367`
- 当前行为：`parseTimeFilter` 对未知格式原样返回，让 SQL 查询自然失败或查不到。
- 影响：用户输入错误时间时可能得到空列表，而不是明确错误。
- 建议：改为返回 `(string, error)`，非法时间格式直接报错。

### 28. hostname 获取失败返回空

- 类型：`silent-ignore`
- 位置：`internal/cli/run.go:452`
- 当前行为：`os.Hostname()` 错误被忽略，返回空字符串。
- 影响：低风险，但记录不完整。
- 建议：记录 warning，或在 metadata 中使用明确的 `unknown`。

### 29. username 直接读取 USER

- 类型：`default`
- 位置：`internal/cli/run.go:457`
- 当前行为：只读 `USER` 环境变量，不存在则为空。
- 影响：在非交互环境或 Windows 上可能为空。
- 建议：使用 `os/user` 或明确记录 `unknown`。

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

- 类型：`degrade`
- 位置：`cmd/web.go:375`, `cmd/resource_linux.go:158`
- 当前行为：Web processes API 先按 `/proc/<pid>/task/<pid>/children` 递归采集进程树；如果没有结果，则退回 `ListProcessGroup(pid)`。
- 影响：退回进程组扫描后可能丢失 depth/role 层级语义，也可能漏掉跨进程组但仍是子孙的进程。
- 建议：响应中增加诊断字段，例如 `process_source=tree|group|empty`，让 UI 和 API 调用方知道当前数据来自哪种采集路径。

### 33. 进程 activity 采样失败时沿用第一帧

- 类型：`degrade`
- 位置：`cmd/resource_linux.go:158`
- 当前行为：`ListProcessTreeWithActivity` 先采第一帧，短暂等待后采第二帧；如果第二帧为空，则返回第一帧并按状态推断 active。
- 影响：`active_count` 和 `cpu_delta_ms` 可能偏保守，用户无法区分“进程确实不活跃”和“采样窗口内进程消失/采样失败”。
- 建议：增加 activity 采样状态，例如 `activity_sampled=true|false` 或 `sample_interval_ms`，并在 UI 摘要中保留诊断信息。

### 34. Last log update 缺失时返回空值

- 类型：`default`
- 位置：`cmd/web.go:727`
- 当前行为：如果 stdout/stderr 都不存在或无法 stat，`last_log_update` 和 `last_log_update_ago` 返回空字符串。
- 影响：UI 会隐藏 Last Log，用户无法区分“没有日志文件”和“日志很久没更新但可读”。
- 建议：返回明确状态，例如 `last_log_status=missing|unreadable|ok`，或在诊断摘要中显示 `no log files`。

### 35. 进程角色无法识别时归为 worker

- 类型：`classification-default`
- 位置：`cmd/resource_linux.go:417`
- 当前行为：进程角色按 root、runner、shell 判断；未命中时默认 `worker`。
- 影响：未知进程会被展示成 worker，可能把 tee、tail、监控辅助进程和真正计算进程混在一起。
- 建议：增加 `process` 或 `unknown` 角色，把默认分类从 worker 调整为 unknown；worker 只用于更明确的计算进程规则。

## 建议清理顺序

1. 先修复会造成用户看到错误标识的 fallback：后台 detached run 预生成 RunID。
2. 再清理已诊断化但仍可能造成数据缺失的 degrade：保存 command/env/script、tag/note、artifact、metadata 的写入失败。
3. 再清理会造成行为漂移的 fallback：CWD 自动推断、project 目录名推断、Web 端口自动递增。
4. 然后清理会掩盖配置错误的 fallback：`brun.yaml` 解析失败、时间过滤解析失败。
5. 最后处理合理但需要显式化的 fallback：非 Linux 资源采样、进程诊断采样状态、Git 信息采集、Conda 信息采集、HomeDir 默认值。

## 保留候选

以下 fallback 当前看起来合理，但仍建议显式记录：

- `BRUN_HOME` 未设置时使用 `$HOME/.bio-runner`。
- SQLite busy 自动重试。
- `brun list` 在数据库不存在时创建空库。
- note 不存在时返回空字符串。
- `--allow-exit` 显式覆盖状态。
