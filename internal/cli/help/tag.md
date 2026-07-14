---
use: "tag [<run_id> | --latest] TAG..."
short: "添加标签"
long: |
  为指定 run 添加标签。标签格式自由文本，推荐使用 key:value 格式便于过滤
  （如 sample:S1, stage:align, hg38）。多次调用追加标签，不覆盖已有标签。
example: |
  brun tag 20260605-145615-fed727 sample:S1 production
  brun tag --latest sample:S1 production
output: |
  ## 标签格式约定

  - 自由文本格式，支持空格（用引号包裹）
  - 推荐 key:value 格式用于结构化标记
  - 支持逗号分隔批量指定：brun run -t align,hg38
  - 通过 brun list -t key:value 按标签过滤

  注意：当前版本不支持删除或修改已有标签。
---
