# 共享概念（作者参考）

本文件供编写各命令帮助文本时参考，不会被嵌入运行时。

## Run ID 格式

每次运行生成唯一标识：`YYYYMMDD-HHMMSS-xxxxxx`（14 位数字时间戳 + 6 位随机 hex）。
示例：`20260605-145615-fed727`

支持 `--latest` 代替显式指定，取创建时间最近的一条记录。

## 全局选项

以下选项对所有命令可用：
- `-h, --help` 显示帮助信息
- `-v, --version` 显示版本号

## 状态值

| 状态 | 说明 |
|------|------|
| `running` | 正在执行 |
| `success` | 成功完成 |
| `failed` | 执行失败（非零退出码） |
| `success_with_warnings` | 成功但诊断链路有 warning |
| `failed_with_warnings` | 失败且诊断链路有 warning |
| `cancelled` | 被用户终止 |
| `cancelled_with_warnings` | 被终止且诊断有 warning |
| `timed_out` | 达到运行超时限制 |
| `timed_out_with_warnings` | 超时且诊断有 warning |

## 时间过滤语法

以下写法等效，适用于 `--since` / `--until` / `--older-than`：

- 绝对日期：`2026-06-05`
- RFC3339：`2026-06-05T01:02:03Z`
- 相对时间：`30d`（30 天）、`2h`（2 小时）、`1w`（1 周）
- 关键词：`today`

## 输出文件分类 (Kind)

| Kind | 说明 | 常见扩展名 |
|------|------|-----------|
| data | 数据文件 | .bam, .vcf, .fastq, .sam, .bam.bai, .cram |
| log | 日志文件 | .log, .o, .er, .err |
| report | 报告文件 | .html, .txt |
| config | 配置文件 | .yaml, .yml, .json, .conf, .xml |
| index | 索引文件 | .fai, .dict, .csi |
| script | 脚本文件 | .sh, .py, .R, .pl |
| other | 其他文件 | 未识别扩展名 |

## BRUN_HOME

默认数据目录为 `~/.brun`，可通过环境变量 `BRUN_HOME` 覆盖。
运行记录存储在 `$BRUN_HOME/runs/YYYY/MM/DD/<run_id>/` 下。
