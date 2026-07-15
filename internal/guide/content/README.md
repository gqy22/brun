# brun 内置经验内容规范

每条经验使用一个 Markdown 文件，路径为 `content/<tool>/<topic>.md`。文件由
YAML frontmatter 和固定章节的 Markdown 正文组成，编译时嵌入 brun 二进制。

## 元数据

必填字段：

- `id`：稳定且全局唯一，格式为 `<tool>.<topic>`。
- `title`、`summary`：面向使用者的标题和一句话结论。
- `tool`、`category`、`tags`、`commands`：用于分类和搜索。
- `level`：`basic`、`intermediate` 或 `advanced`。
- `status`：`draft`、`tested`、`verified`、`benchmarked` 或 `deprecated`。
- `tool_versions.tested`：实际运行过的工具版本。
- `tool_versions.applicable`：根据官方文档判断的适用版本范围。
- `updated`：最后验证日期，格式为 `YYYY-MM-DD`。

`verified` 表示命令已跑通，并检查了优化前后结果；`benchmarked` 还要求保存可复现的
性能测试环境和结果。没有实际验证的内容不能使用这两个状态。

## 正文章节

正文必须依次包含：

1. `结论`
2. `适用场景`
3. `推荐命令`
4. `为什么这样做`
5. `并行与资源`
6. `注意事项`
7. `结果检查`
8. `依据`

命令中的可变值使用 `{input_vcf}`、`{output_vcf}`、`{contig}`、`{threads}` 等带花括号
的语义化占位符。正文必须区分官方行为、实践建议和特定环境下的实测结果。

