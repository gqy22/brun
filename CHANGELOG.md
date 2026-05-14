# 更新日志

这个文件记录项目中值得关注的版本变化。

项目使用带 `v` 前缀的语义化版本标签，例如 `v0.1.0`。

## [Unreleased]

## [0.2.0] - 2026-05-14

### 新增

- **Web Dashboard** (`brun web`)：完整的可视化管理界面
  - Dashboard 首页：表格视图 + 移动端卡片视图，支持按项目/状态/标签/关键词过滤，底部统计面板（总运行/成功率/今日运行），running 任务自动 10s 刷新
  - 任务详情页：左右分栏布局 — 左侧信息面板（状态/耗时/资源消耗/命令/Git/时间/备注），右侧终端风格深色日志查看器（stdout/stderr 切换、搜索高亮、tail 截断、auto-refresh）+ 输出文件列表
  - 任务操作：终止运行中任务、删除已完成记录、重跑（生成全新 Run 记录并跳转）、复制命令
  - 局域网访问：启动时自动检测并打印所有可用 IP 地址
  - Toast 通知 + Modal 确认对话框
  - 日志首次加载打字机逐行淡入动画效果
- **资源监控**：任务结束时自动从 `/proc/{pid}/` 采集峰值内存（VmHWM, KB）和 CPU 累计时间（utime+stime, ms），Web 详情页直接展示
- **健康检查循环**：后台定时扫描 running 任务，进程已死则自动修正为 failed 状态
- **任务终止**：`SIGTERM → SIGKILL` 两阶段优雅终止，终止后也采集一次资源快照
- **任务删除**：级联删除 artifacts/tags/notes 记录及运行目录
- **智能 CWD 检测**：脚本文件自动使用其所在目录作为工作目录
- **日志增强**：stdout/stderr 分离存储、`--follow` 实时跟踪、`--tail N` 截尾查看、`--stdout`/`--stderr` 切换流
- **前端视觉重构**：DM Sans 字体、navbar 毛玻璃模糊效果、粘性表头、空状态插画、filter bar 图标前缀、staggered 行淡入动画

### 修复

- 终止任务时先探活进程（signal 0），已死则自动修正为 failed 而非报错
- 删除任务 SQL 错误：runs 表主键是 id 非 run_id
- 日志搜索高亮显示为原始 `<mark>` 标签文本（esc 先于 mark 插入）
- CreateRun INSERT 占位符数量与列数对齐（21 列需 21 个占位符）
- DB 迁移幂等化：ALTER TABLE ADD COLUMN 重复执行不再报错

### 变更

- Web 重跑功能从裸 `exec.CommandContext` 升级为完整流程（新建 RunID / 写 DB / 记录日志 / 采集资源 / 更新状态）

### 工具链

- CI Verify 增加 gofmt 格式检查门禁
- 四平台交叉编译：linux/amd64、linux/arm64、darwin/amd64、darwin/arm64

## [0.1.0] - 2026-05-13

### 新增
- **`brun run`**：核心命令，包装任意外部命令并自动记录运行信息。支持 `--name`/`--project`/`--tag`/`--note`/`--output`/`--timeout`/`--cwd` 等参数
- **`brun list`**：列出运行历史，支持按 `--project`/`--status`/`--tag` 过滤和 `--limit` 限制数量
- **`brun show`**：显示运行详情，包含命令、状态、耗时、退出码、Git 信息、Tags、Note
- **`brun logs`**：查看 stdout/stderr 日志，支持 `--stdout`/`--stderr`/`--tail N`/`--follow`
- **`brun outputs`**：查看输出文件列表，通过文件系统 before/after diff 自动捕获新增/修改/删除文件
- **`brun tag` / `brun note`**：为运行添加标签和备注
- **`brun rerun`**：重新执行历史运行的原始命令，支持 `--dry-run`/`--cwd`/`--with-same-tags`
- **`brun init`**：在当前目录生成 `brun.yaml` 项目配置模板
- **`brun clean`**：清理旧运行记录（dry-run 模式）
- **run_id 生成**：格式 `YYYYMMDD-HHMMSS-xxxxxx`，全局唯一
- **SQLite 存储**：4 张表（runs/artifacts/tags/notes），支持 10 万+ run 记录
- **Git 信息采集**：自动记录 repo、branch、commit、dirty 状态
- **环境摘要**：捕获 PATH/HOME/USER/SHELL/LANG/CONDA 等关键环境变量
- **文件系统 diff**：运行前后快照对比，自动发现新增/修改/删除文件
- **Artifact 分类**：按扩展名自动分类为 script/config/report/input/output
- **大文件策略**：默认 >50MB 标记为大文件，只记录元数据不复制
- **metadata.yaml**：每次运行生成结构化 YAML 元数据文件
- **stdout/stderr 双写**：实时输出到终端同时写入日志文件
- **超时控制**：`--timeout` 参数支持命令超时自动终止
- **信号转发**：Ctrl+C 正确转发给子进程并记录 interrupted 状态

### 变更
- 项目识别优先级：CLI `--project` > `brun.yaml` > Git repo 名 > 目录名

### 文档
- PRD 完整产品需求文档
- README 快速开始与命令参考
- Makefile 编译/测试/打包/交叉编译/UPX 压缩
- GitHub Actions release 工作流（verify → build → publish）
