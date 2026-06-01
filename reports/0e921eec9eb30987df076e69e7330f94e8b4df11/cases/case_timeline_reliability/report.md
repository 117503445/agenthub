# Timeline 可靠性 E2E 测试报告

- 结果: 通过
- 日志: [test.log](logs/test.log)
- 服务日志: [server.log](logs/server.log)

## 过程

- state.snapshot 不再携带全局游标，前端通过 chat.timeline.fetch 初始化聊天 timeline。
- chat.timeline.fetch after 可按 per-chat epoch/seq 补齐断线期间错过的行。
- chat.timeline.fetch before 可拉取更早窗口，支持长聊天分页。
- gap 或 epoch 不匹配时，后端返回 canonical reset 窗口，前端可安全重建本地 timeline。
