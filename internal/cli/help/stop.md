---
use: "stop <run_id>"
short: "终止运行中的任务"
long: |
  向运行中的任务发送终止信号（SIGTERM），等待优雅退出后强制结束（SIGKILL）。
  终止的是整个进程组，包括所有子进程。任务状态会自动更新为 failed。
example: |
  brun stop 20260605-145615-fed727
  brun stop --latest
  brun stop --latest --force
output: |
  ## 行为说明

  默认分两阶段终止：
  1. 发送 SIGTERM，等待 **10 秒**宽限期让进程优雅退出
  2. 宽限期过后发送 SIGKILL 强制结束

  使用 --force (-f) 跳过宽限期，直接发送 SIGKILL。

  对已结束（success/failed/cancelled）的 run 执行 stop 会报错。
---
