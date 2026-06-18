---
use: "diag <run_id>"
short: "查看运行诊断"
long: |
  查看运行诊断。默认只显示 warning/error；使用 --all 显示 info/warning/error；
  使用 --json 输出机器可读结果。
example: |
  brun diag 20260605-145615-fed727
  brun diag --latest
  brun diag --latest --all
  brun diag --latest --json
---
