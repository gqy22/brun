---
use: "init [名称]"
short: "在当前目录生成脚本模板"
long: |
  生成带标准注释头部的脚本模板。名称默认为 script。
  自动检测 conda 环境、计算编号（同名递增后缀）。
example: |
  # 生成 01_align.sh
  brun init align

  # 同名再次生成 → 01_align2.sh
  brun init align

  # 不同名 → 新编号
  brun init call          # → 02_call.sh
---
