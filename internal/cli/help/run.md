---
use: "run -- <command...>"
short: "执行命令并记录运行信息 (默认 nohup 后台运行)"
long: |
  执行命令并自动记录运行日志、环境信息、Git 状态和输出文件变更。默认以 nohup 方式后台运行，关闭终端不会中断任务。
example: |
  # 基本用法 (默认 nohup 后台运行，关终端不会中断)
  brun run -- bwa mem -t 16 ref.fa reads_*.fq > aligned.sam
  # 日志写入: ~/.brun/runs/YYYY/MM/DD/<run_id>/stdout.o 和 stderr.er

  # 带项目名和标签
  brun run -p genome-align -t hg38,pep-align -- bwa mem ref.fa reads.fq > aligned.sam

  # 智能体推荐：前台运行，命令退出后再读取 run 记录和诊断
  brun run -f -p genome-align -n align-S1 -- bwa mem ref.fa reads.fq

  # Snakemake 流程 (前台运行，方便调试)
  brun run -f -- snakemake -j 8

  # FastQC 质控，指定名称和备注
  brun run -n "qc-report" --note "样本质量控制" -- fastqc *.fastq.gz

  # samtools 允许特定非零退出码（如空区域）
  brun run --allow-exit 1,2 -- samtools view -b input.bam "chr1:1-1000"

  # 在指定目录运行 R 脚本
  brun run --cwd /data/project -- Rscript analysis.R

  # 设置超时 (1 小时)
  brun run --timeout 3600 -- hisat2 -x genome_idx -1 r1.fq -r2.fq
output: |
  ## 输出

  **后台模式**：打印 run ID（格式 YYYYMMDD-HHMMSS-xxxxxx）后立即返回，命令在后台继续执行。
  **前台模式** (-f)：透传命令退出码，命令结束后再写入 run 记录。

  日志文件位置：~/.brun/runs/YYYY/MM/DD/<run_id>/stdout.o 和 stderr.er
---
