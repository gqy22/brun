---
use: "show [<run_id> | --latest]"
short: "显示运行详情"
long: |
  显示单次运行的完整元数据详情，包括执行信息、资源消耗、Git 状态、标签备注和诊断结果。
example: |
  brun show 20260605-145615-fed727
  brun show --latest
output: |
  ## 输出字段

  **基本信息**: Run ID, Name, Project, Command, Status
  **路径与时间**: CWD, StartedAt, EndedAt, Duration
  **退出码**: ExitCode (0=成功, 非零=失败)
  **资源消耗**: PeakRSSKB (内存峰值), CPUTimeMs (CPU 时间)
  **Git 信息**: GitRepo, GitCommit (短 SHA 前 8 位), GitDirty (是否有未提交更改)
  **Conda/Python**: CondaEnv, PythonVersion
  **标签与备注**: Tags (列表), Note (自由文本)
  **诊断**: Info/Warning/Error 计数, 最后一条诊断的 Code/Message/Time

  使用 --json 输出完整 JSON 对象，包含所有上述字段。
---
