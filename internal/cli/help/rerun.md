---
use: "rerun <run_id>"
short: "重新运行"
long: |
  基于已有 run 记录重新执行相同命令。默认继承原 run 的 project 和 cwd；
  使用 --cwd 可覆盖运行目录；使用 --with-same-tags 继承原标签到新 run；
  使用 --name 指定新 run 的名称。--dry-run 只打印不执行。
example: |
  brun rerun 20260605-145615-fed727 --dry-run
  brun rerun --latest --dry-run
  brun rerun --latest --cwd /data/project
output: |
  ## 继承规则

  | 属性 | 默认行为 | 可覆盖方式 |
  |------|----------|-----------|
  | 命令 (Command) | 继承原命令 | 不可覆盖 |
  | 项目 (Project) | 继承 | — |
  | 工作目录 (CWD) | 继承 | --cwd |
  | 标签 (Tags) | 不继承 | --with-same-tags |
  | 名称 (Name) | 不继承 | --name |

  新 run 会获得全新的 run ID，原 run 记录不受影响。
---
