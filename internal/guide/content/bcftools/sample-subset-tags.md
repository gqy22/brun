---
schema: 2
id: bcftools.sample-subset-tags
title: 样本子集后更新 AC 与 AN
tool: bcftools
category: quality
kind: pitfall
summary: 使用 view 提取样本时默认更新 AC/AN；需要按更新后的频率筛选时，把样本提取放在前一个管道阶段。
tags:
  - samples
  - ac
  - an
  - filtering
commands:
  - view
  - query
level: intermediate
status: verified
versions:
  tested:
    - "1.22.1"
  documented:
    - "1.22"
  notes: 本条验证 view 对 INFO/AC 和 INFO/AN 的默认更新行为。
evidence:
  docs:
    - title: BCFtools manual
      url: https://samtools.github.io/bcftools/bcftools
      checked: "2026-07-15"
  validations:
    - bcftools.sample-subset-tags
  benchmarks: []
updated: "2026-07-15"
---

## 结论

`bcftools view -s/-S` 提取样本时，默认会根据保留样本更新 `INFO/AC` 和 `INFO/AN`。如果后续
筛选依赖更新后的等位基因计数，应先完成样本子集，再通过 `-Ou` 管道执行下一次筛选。

## 适用场景

- 从队列 VCF 中提取一个人群或样本集合。
- 根据子集中的 AC、AN 或 AF 再次筛选位点。
- 核查样本子集前后 INFO 统计是否仍代表当前样本。

## 推荐方法

只提取样本并保留更新后的 AC/AN：

```bash
bcftools view -s "{samples}" -Oz -o "{output_vcf}" "{input_vcf}"
```

按子集更新后的 AC 继续筛选：

```bash
bcftools view -s "{samples}" -Ou "{input_vcf}" |
  bcftools view -i 'INFO/AC>0' -Oz -o "{output_vcf}"
```

## 为什么这样做

样本减少后，原队列的 AC/AN 不再代表输出中的基因型。`view` 会更新这些标签，但同一条命令
中的筛选通常先于样本子集发生；拆成两个流式阶段能明确保证第二阶段看到更新后的值。

## 注意事项

- `-I/--no-update` 会禁止 AC/AN 更新，除非明确需要保留原队列统计，否则不要使用。
- 并非所有自定义 INFO 标签都会自动重算；应了解每个标签的来源。
- 子集后的 AF 可以由更新后的 AC/AN 计算，但仍需处理缺失基因型和倍性。
- 样本名不存在时默认行为和 `--force-samples` 需要谨慎确认。

## 结果检查

同时查看基因型与 AC/AN：

```bash
bcftools query -f '%CHROM\t%POS\t%INFO/AC\t%INFO/AN[\t%SAMPLE=%GT]\n' \
  "{output_vcf}"
```

## 依据

- 官方行为：bcftools 手册说明 `view` 在样本子集后更新 AC/AN，并建议用管道让后续命令读取
  更新值。
- 本地验证：bcftools 1.22.1 对样本 A 提取后，`idSNP` 从 `AC=1,1;AN=4` 更新为
  `AC=1,0;AN=2`；使用 `-I` 时保持原值。
- 官方文档：https://samtools.github.io/bcftools/bcftools
- 验证 ID：`bcftools.sample-subset-tags`。
