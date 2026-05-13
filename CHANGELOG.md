# 更新日志

这个文件记录项目中值得关注的版本变化。

项目使用带 `v` 前缀的语义化版本标签，例如 `v0.1.0`。

## [Unreleased]

### 新增

### 修复

### 变更

### 性能

### 移除

### 工具链

### 文档

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
