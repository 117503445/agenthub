# Chat append-only timeline E2E 测试报告

- 结果: 通过
- 日志: [test.log](logs/test.log)
- 服务日志: [server.log](logs/server.log)

## 过程

- state.snapshot 只包含聊天摘要，正文需要通过 timeline 拉取。
- 空聊天可拉取 tail 窗口并初始化 per-chat epoch/seq。
- 发送消息后服务端通过 chat.timeline.appended 推送单调递增的 canonical row。
- after 可按游标补齐断线期间新增 timeline 行。
- before 可按游标拉取更早窗口，支持长聊天分页。
- epoch 不匹配时，聊天 timeline 返回 canonical reset 窗口。
- 后端重启后从 timelines/<chatID>.json 恢复 canonical rows。
