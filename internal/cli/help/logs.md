---
use: "logs [<run_id> | --latest]"
short: "查看运行日志"
long: |
  查看运行日志。支持 --follow 实时跟踪输出（类似 tail -f）。
  普通模式默认同时输出 stdout 和 stderr；可通过互斥的 --stdout / --stderr 单独筛选。
  --follow 模式默认跟踪 stdout，可使用 --stderr 改为跟踪 stderr。
example: |
  # 查看最新运行的日志
  brun logs --latest

  # 实时跟踪正在运行的命令输出
  brun logs --latest -f

  # 只看最后 50 行 stderr
  brun logs <run_id> --stderr --tail 50
---
