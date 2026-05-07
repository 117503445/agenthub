# 聊天详情懒加载 E2E 测试报告

- 结果: 通过
- 日志: [test.log](logs/test.log)
- 服务日志: [server.log](logs/server.log)

## 过程

- 通过 WebSocket 在首个聊天页中写入一条历史消息。
- 重新连接后的 state.snapshot 只返回聊天页摘要，不携带历史消息和 plan 详情。
- AGENTHUB_DATA/state.json 只保存聊天摘要，聊天详情写入 chats/<chatID>.json。
- 前端请求 chat.detail.get 后，后端才返回该聊天页的完整消息详情。
- 浏览器直接进入聊天页 hash 路由后，前端请求详情并渲染历史消息；没有滚动记录时默认停在底部。
