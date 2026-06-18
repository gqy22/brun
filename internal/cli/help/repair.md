---
use: "repair"
short: "从 run 目录重建 SQLite 索引"
long: |
  扫描 runs 目录下的 metadata.yaml 文件，重建 SQLite run 索引。
  默认只预览发现的缺失记录，使用 --write 才会实际写入数据库。
  适用于异常关机、手动删除数据库等导致索引损坏的场景。
example: |
  # 预览可恢复的记录
  brun repair

  # 写入缺失的索引
  brun repair --write
---
