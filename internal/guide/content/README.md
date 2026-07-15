# brun 内置经验内容规范 v2

每条经验使用一个 Markdown 文件，路径为 `content/<tool>/<topic>.md`。文件由严格的 YAML
frontmatter 和 Markdown 正文组成，编译时嵌入 brun 二进制。

## 元数据

必填字段：

- `schema`：当前固定为 `2`。
- `id`：稳定且全局唯一，格式为 `<tool>.<topic>`。
- `title`、`summary`：面向使用者的标题和一句话结论。
- `tool`、`category`、`tags`、`commands`：分类和搜索信息。
- `kind`：内容形式，取值为 `practice`、`comparison`、`workflow`、`performance`、
  `pitfall` 或 `troubleshooting`。
- `level`：`basic`、`intermediate` 或 `advanced`。
- `status`：`draft`、`reviewed`、`verified`、`benchmarked` 或 `deprecated`。
- `versions.tested`：实际运行验证过的版本。
- `versions.documented`：核对过官方文档的版本，不代表逐版本实测。
- `evidence`：官方文档、验证案例和基准报告的结构化引用。
- `updated`：最后验证日期，格式为 `YYYY-MM-DD`。

不要使用未经逐版本验证的宽泛适用范围，例如只测试 `1.22.1` 时不能直接声称适用于
`>=1.18`。版本推断应写入 `versions.notes`，并明确其证据边界。

## 状态

```text
draft       内容尚未完成
reviewed    已核对官方文档和 documented 版本
verified    reviewed，并有正确性案例和 tested 版本
benchmarked verified，并有结构化性能报告
deprecated  已不再推荐
```

解析器会根据状态检查必要证据，不能仅修改 `status` 提升可信级别。

## 正文章节

所有内容只强制以下五个核心章节，并保持此相对顺序：

1. `结论`
2. `适用场景`
3. `推荐方法`
4. `注意事项`
5. `依据`

可以根据主题在核心章节之间加入：

- `推荐命令`
- `为什么这样做`
- `并行与资源`
- `结果检查`
- `性能数据`
- `常见错误`

不相关的可选章节不要为了模板完整而填充空话。

## 证据

示例：

```yaml
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-14"
  validations:
    - bcftools.pipeline-equivalence
  benchmarks:
    - bcftools.pipeline-2026-07-14
```

- `docs` 记录官方资料和实际核对日期。
- `validations` 必须对应 `guide/cases/` 中存在的案例。
- `benchmarks` 必须对应 `guide/reports/` 中存在的结构化报告。
- 正文中的“依据”负责向用户解释，frontmatter 负责自动校验。

## 命令占位符

可变值使用 `{input_vcf}`、`{output_vcf}`、`{contig}`、`{threads}` 等语义化占位符。
正文必须区分官方行为、实践建议和特定环境下的实测结果。
