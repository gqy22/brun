---
use: "clean --older-than <duration>"
short: "清理旧运行记录"
long: |
  清理符合条件的 run 记录和对应的 run 目录。默认只预览匹配结果，
  必须显式使用 --write 才会实际删除。建议先不加 --write 预览确认后再执行。
example: |
  # 预览 30 天前的旧记录（不会删除）
  brun clean --older-than 30d

  # 预览并保留失败 run + JSON 输出
  brun clean --older-than 30d --keep-failed --json

  # 实际删除 90 天前的旧记录
  brun clean --older-than 90d --write
output: |
  ## 时间格式

  --older-than 支持：30d (天), 12h (小时), 2w (周), 以及绝对日期 YYYY-MM-DD。

  ## 清理策略

  按 run 的创建时间判断，早于指定时长的记录会被匹配。
  匹配到的记录在预览模式下显示摘要（数量、总大小、ID 列表），
  加 --write 后逐条删除 SQLite 记录和对应目录。
---
