---
use: "list"
short: "列出运行历史"
long: |
  列出运行历史记录。支持按项目、状态、标签、关键词搜索和时间范围过滤。
  默认显示最近 20 条，按创建时间降序排列。
example: |
  # 查看最近 20 条运行记录
  brun list

  # 按项目过滤
  brun list -p genome-align

  # 只看失败的运行
  brun list -S failed

  # 搜索关键词（匹配命令或名称）
  brun list -s bwa

  # 最近一周的记录
  brun list --since 1w

  # 指定时间范围
  brun list --since 2024-01-01 --until 2024-03-31

  # 按主机/用户过滤
  brun list --host compute-node-01 --user bioinfo

  # JSON 输出（适合管道处理）
  brun list --json
output: |
  ## 输出格式

  | 列名 | 说明 |
  |------|------|
  | RUN ID | 唯一标识 (YYYYMMDD-HHMMSS-xxxxxx) |
  | NAME | 用户指定的名称 |
  | PROJECT | 项目名 |
  | STATUS | 运行状态 (见下方状态值说明) |
  | DIAG | 诊断摘要 (E=N错误 W=N警告 -=无诊断) |
  | DURATION | 运行时长 |
  | COMMAND | 执行的命令（截断显示） |

  状态值包含基础状态 (running/success/failed/cancelled) 及其 warnings 变体，
  如 success_with_warnings 表示命令成功但诊断链路存在 warning。
---
