---
use: "note [<run_id> | --latest] <text>"
short: "添加备注"
long: |
  为指定 run 添加自由文本备注。每次调用会**覆盖**之前的备注（单条备注模式），
  而非追加。适合记录运行目的、参数说明或实验结论。
example: |
  brun note 20260605-145615-fed727 "STAR index 参数测试"
  brun note --latest "STAR index 参数测试"
---
