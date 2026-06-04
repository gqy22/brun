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

### 4. brun.yaml 解析失败被忽略

- 类型：`silent-ignore`
- 位置：`main.go:272`
- 当前行为：读取到 `brun.yaml` 后，`ParseConfig` 的错误被忽略，继续使用零值配置。
- 影响：配置写错时用户无感，ignore/capture/project 都可能不生效。
- 建议：删除该 fallback。存在 `brun.yaml` 但解析失败时应直接报错。

### 5. Git 信息采集失败被忽略

- 类型：`silent-ignore`
- 位置：`internal/git.go:72`
- 当前行为：不是 Git repo 或 git 命令失败时，repo/branch/commit 留空。
- 影响：对非 Git 目录友好，但 git 命令异常、权限问题、坏 repo 也会被吞掉。
- 建议：区分“不是 Git repo”和“Git repo 但采集失败”。前者可以允许，后者应记录 warning 或报错。

### 6. 命令工作目录自动推断

- 类型：`infer`
- 位置：`main.go:1189`
- 当前行为：如果首个参数是已存在脚本文件，使用脚本所在目录；否则回退当前目录。
- 影响：行为不透明。同一个命令在脚本路径存在或不存在时 CWD 不同，可能造成输出文件位置变化。
- 建议：后续优先使用显式 `--cwd`。自动推断可以删除，或只在 `brun run-script <path>` 这类专门命令中启用。

### 7. 脚本快照自动查找失败后跳过

- 类型：`silent-ignore`
- 位置：`main.go:297`, `main.go:1222`
- 当前行为：尝试在参数中查找脚本文件；找不到、文件不存在、不是文本文件时直接不保存快照。
- 影响：用户可能以为脚本已被记录，实际没有。
- 建议：如果命令形态明显包含脚本但快照失败，应提示 warning。也可以提供 `--script` 显式指定快照文件。

### 8. 保存 command/env/script 失败被忽略

- 类型：`silent-ignore`
- 位置：`main.go:294`
- 当前行为：`SaveCommandFile`、`SaveEnvFile`、`SaveInputScript` 返回错误未检查。
- 影响：关键审计文件可能缺失，但 run 仍继续执行。
- 建议：删除该 fallback。`command.sh` 和 `env.txt` 保存失败应阻断运行；脚本快照失败至少 warning。

### 9. 文件系统 before/after 快照失败被忽略

- 类型：`silent-ignore`
- 位置：`main.go:336`, `main.go:358`
- 当前行为：`SnapshotDir` 错误被忽略，fs-diff 继续执行。
- 影响：artifact 记录可能为空或不完整，用户无感。
- 建议：`--no-fs-diff` 是显式关闭；默认开启时快照失败应报错或至少输出 warning，并在 run metadata 中记录。

### 10. 遍历目录时单文件错误被忽略

- 类型：`silent-ignore`
- 位置：`internal/capture.go:36`
- 当前行为：`filepath.Walk` 遇到单个文件错误时 `return nil`，继续遍历。
- 影响：权限错误、坏链接、瞬时删除都不会进入结果，也没有提示。
- 建议：收集 skipped files，返回附带 warning；或在严格模式下报错。

### 11. Artifact 写入失败被忽略

- 类型：`silent-ignore`
- 位置：`main.go:363`
- 当前行为：`store.CreateArtifact` 返回错误未检查。
- 影响：输出文件检测到了但未入库，用户无感。
- 建议：至少记录 warning；如果 artifact 是核心能力，应该失败时更新 run metadata 或报错。

### 12. tag/note 写入失败被忽略

- 类型：`silent-ignore`
- 位置：`main.go:405`
- 当前行为：`store.AddTag`、`store.AddNote` 返回错误未检查。
- 影响：用户传入的标签和备注可能丢失。
- 建议：删除该 fallback，写入失败应报错。

### 13. 资源数据更新失败被忽略

- 类型：`silent-ignore`
- 位置：`main.go:427`
- 当前行为：`UpdateRunResources` 返回错误未检查。
- 影响：资源数据丢失但用户无感。
- 建议：保留运行状态更新优先级，但资源写入失败应 warning。

### 14. metadata.yaml 写入失败被忽略

- 类型：`silent-ignore`
- 位置：`main.go:436`
- 当前行为：`os.WriteFile(metadata.yaml)` 返回错误未检查。
- 影响：run 目录中的离线元数据可能缺失。
- 建议：删除该 fallback，或至少 warning 并记录到日志。

### 15. 失败摘要读取 stderr 失败被忽略

- 类型：`silent-ignore`
- 位置：`main.go:442`
- 当前行为：读取 stderr 失败时不显示摘要。
- 影响：低风险，只影响失败后的辅助展示。
- 建议：可以保留，但最好打印“无法读取 stderr 摘要”的提示。

### 16. allow-exit 把失败改成功

- 类型：`default`
- 位置：`main.go:416`
- 当前行为：如果 exit code 在 `--allow-exit` 中，最终状态从 `failed` 改成 `success`。
- 影响：这是显式用户配置，不属于坏 fallback，但会改变真实失败语义。
- 建议：保留，但 metadata 中应同时记录真实 exit code 和 status override reason。

### 17. 后台 detached run 预生成 runID

- 类型：`default`
- 位置：`main.go:463`
- 当前行为：后台父进程生成一个 runID 用于 stdout/stderr 路径；子进程真正执行时会再次生成自己的 runID。
- 影响：用户看到的 `[nohup] RunID` 可能不是数据库中的实际 run id，这是比 fallback 更严重的语义不一致。
- 建议：优先修复。父子进程应共享同一个 run id，或父进程不要打印 run id。

### 18. Web addr 和 port 默认值

- 类型：`default`
- 位置：`main.go:1161`
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
- 位置：`main.go:1102`
- 当前行为：`parseTimeFilter` 对未知格式原样返回，让 SQL 查询自然失败或查不到。
- 影响：用户输入错误时间时可能得到空列表，而不是明确错误。
- 建议：改为返回 `(string, error)`，非法时间格式直接报错。

### 28. hostname 获取失败返回空

- 类型：`silent-ignore`
- 位置：`main.go:1177`
- 当前行为：`os.Hostname()` 错误被忽略，返回空字符串。
- 影响：低风险，但记录不完整。
- 建议：记录 warning，或在 metadata 中使用明确的 `unknown`。

### 29. username 直接读取 USER

- 类型：`default`
- 位置：`main.go:1182`
- 当前行为：只读 `USER` 环境变量，不存在则为空。
- 影响：在非交互环境或 Windows 上可能为空。
- 建议：使用 `os/user` 或明确记录 `unknown`。

### 30. README 和代码端口默认值曾不一致

- 类型：`documentation-fallback`
- 位置：`README.md:119`, `main.go:1146`
- 当前行为：已修正为 README、代码和 help 都使用 `9213`。
- 影响：历史文档曾写成 `9313`，用户按旧文档访问会失败。
- 建议：保持 README、CLI help 和代码默认值同步；如果后续调整默认端口，三处必须一起更新。

## 建议清理顺序

1. 先清理会造成数据丢失且用户无感的 fallback：保存 command/env/script、tag/note、artifact、metadata 的错误忽略。
2. 再清理会造成行为漂移的 fallback：CWD 自动推断、project 目录名推断、Web 端口自动递增。
3. 然后清理会掩盖配置错误的 fallback：`brun.yaml` 解析失败、时间过滤解析失败。
4. 最后处理合理但需要显式化的 fallback：非 Linux 资源采样、Git 信息采集、Conda 信息采集、HomeDir 默认值。

## 保留候选

以下 fallback 当前看起来合理，但仍建议显式记录：

- `BRUN_HOME` 未设置时使用 `$HOME/.bio-runner`。
- SQLite busy 自动重试。
- note 不存在时返回空字符串。
- `--allow-exit` 显式覆盖状态。
