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

# 运行并记录
brun run --name align-S1 --tag rnaseq -- bwa mem -t 16 ref.fa reads.fq.gz

# 查看记录
brun list
brun show latest
brun logs latest --tail 100
brun outputs latest

# 添加备注和标签
brun tag latest important failed-debug
brun note latest "STAR index 参数测试，内存占用偏高"

# 复跑
brun rerun latest --dry-run
```

## 命令一览

| 命令 | 说明 |
|------|------|
| `brun run -- <cmd>` | 执行命令并完整记录 |
| `brun list` | 列出运行历史 |
| `brun show <id\|latest>` | 查看运行详情 |
| `brun logs <id\|latest>` | 查看日志 |
| `brun outputs <id\|latest>` | 查看输出文件 |
| `brun tag <id> TAG...` | 添加标签 |
| `brun note <id> "text"` | 添加备注 |
| `brun rerun <id\|latest>` | 重新运行 |
| `brun init` | 生成 brun.yaml |
| `brun clean` | 清理旧记录 |

## brun run 参数

```bash
brun run [options] -- <command...>
```

| 参数 | 说明 |
|------|------|
| `--name NAME` | 为 run 指定可读名称 |
| `--project PROJECT` | 手动指定项目名 |
| `--tag TAG` | 添加标签，可重复 |
| `--note TEXT` | 添加备注 |
| `--output PATH/GLOB` | 显式声明输出文件 |
| `--no-fs-diff` | 禁用文件系统 diff |
| `--timeout SECONDS` | 超时时间 |
| `--cwd DIR` | 指定运行目录 |

## 数据存储

默认存储在 `~/.bio-runner/`，可通过 `BRUN_HOME` 环境变量覆盖：

```text
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
            └── outputs.json      # 输出文件索引
```

## 项目配置 (brun.yaml)

```bash
brun init my-project
```

生成的 `brun.yaml` 可自定义捕获规则、忽略模式等。

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

## License

MIT
