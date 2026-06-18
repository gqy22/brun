---
use: "web"
short: "启动 Web Dashboard（局域网访问）"
long: |
  在本地启动 HTTP 服务，通过浏览器管理运行记录、查看日志和诊断信息。
  默认监听 0.0.0.0:9213；未显式指定 --port 时会自动避开占用端口递增尝试，
  显式指定 --port 时端口不可用会直接报错。
example: |
  # 启动 Web Dashboard（自动选端口）
  brun web

  # 指定端口；如果端口被占用会直接失败
  brun web --port 9090

  # 明确局域网监听地址
  brun web --addr 0.0.0.0
output: |
  ## Web Dashboard 功能

  - 运行历史列表（支持按项目/状态/时间过滤）
  - 运行详情查看（等同于 brun show）
  - 日志实时查看（等同于 brun logs -f）
  - 诊断信息展示（等同于 brun diag）
  - 终止运行中任务（等同于 brun stop）

  启动后会打印访问地址，通常为 http://<hostname>:9213
---
