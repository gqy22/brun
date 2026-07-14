---
use: "script [<run_id> [<other_run_id>] | --latest]"
short: "查看运行时保存的脚本快照"
long: |
  查看运行时自动保存的输入脚本快照。当通过 brun run 执行 .sh/.py/.R 等脚本文件时，
  会自动保存脚本的完整内容副本，即使后续修改了原始脚本也不影响已保存的快照。

  如果没有脚本文件快照（如直接执行 python3 script.py 等命令），会自动回退到 command.sh
  并显示警告标注。输出前会显示运行元信息（状态、时长、退出码、工作目录）。
  长脚本内容会自动调用分页器（$PAGER 或 less）显示。

  支持双 id 对比模式：brun script id1 id2 输出两个脚本的 unified diff。
example: |
  brun script --latest
  brun script 20260522-153012-a8f3c2
  brun script --latest --path

  # 编辑器打开最新快照
  vim $(brun script --latest --path)

  # 对比两次运行的脚本差异
  diff <(brun script id1 --path) <(brun script id2 --path)

  # 双 id 直接对比（内置 diff）
  brun script id1 id2
output: |
  ## 输出

  **单 id 模式**（默认）:
  - 元信息头部栏（文件名、大小、状态、时长、退出码、CWD）
  - fallback 时显示 ⚠ 警告行
  - 脚本完整内容（长内容自动分页）

  **双 id 模式**（对比）:
  - unified diff 格式输出，文件标签含 run_id 前缀

  **--path 模式**:
  - 仅输出快照文件磁盘路径，适合管道给 vim/diff 等外部工具
---
