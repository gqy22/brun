---
use: "brun"
short: "bio-runner: 面向生物信息学的运行记录与日志管理工具"
example: |
  # 快速上手：后台运行命令
  brun run -- bwa mem -t 16 ref.fa reads.fq > aligned.sam

  # 查看运行历史
  brun list

  # 查看最新运行的详情
  brun show --latest

  # 查看实时日志
  brun logs --latest -f

  # 初始化脚本模板
  brun init align
---
brun 是一个跨项目运行记录工具。
通过 `brun run -- <command>` 包装任意命令，自动记录日志、环境、Git 信息和输出文件。
也可以使用 `brun -- <command>` 作为快捷入口，等价于默认后台运行。

默认数据目录为 ~/.brun，可通过 BRUN_HOME 覆盖。
查询最新 run 使用显式 --latest；位置参数始终按真实 run_id 处理。
