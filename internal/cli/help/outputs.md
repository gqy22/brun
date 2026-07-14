---
use: "outputs [<run_id> | --latest]"
short: "查看输出文件"
long: |
  列出运行产生的输出文件。通过 fs-diff 检测命令执行前后的文件系统变更，
  自动识别新增和修改的数据文件、日志、报告等。
example: |
  brun outputs 20260605-145615-fed727
  brun outputs --latest
output: |
  ## 输出格式

  | 列名 | 说明 |
  |------|------|
  | KIND | 文件分类 (data/log/report/config/index/script/other) |
  | STATUS | created(新建) / modified(修改) / deleted(删除) |
  | SIZE | 文件大小 (人类可读格式) |
  | PATH | 相对于运行目录的文件路径 |

  文件分类规则根据常见生信工具的扩展名自动推断（如 .bam→data, .log→log），
  未识别扩展名归入 other。使用 run 时加 --no-fs-diff 可禁用此检测。
---
