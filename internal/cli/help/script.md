---
use: "script <run_id>"
short: "查看运行时保存的脚本快照"
long: |
  查看运行时自动保存的输入脚本快照。当通过 brun run 执行 .sh/.py/.R 等脚本文件时，
  会自动保存脚本的完整内容副本，即使后续修改了原始脚本也不影响已保存的快照。
example: |
  brun script --latest
  brun script 20260522-153012-a8f3c2
  brun script --latest --path
output: |
  ## 输出

  默认输出脚本完整内容。使用 --path 仅输出快照文件的磁盘路径，
  适合传递给其他程序处理（如编辑器打开、diff 对比）。
---
