# WebSocket 自动重连 E2E 测试报告

- 结果: 通过
- 日志: [test.log](logs/test.log)
- 服务日志: [server.log](logs/server.log)

## 过程

- 页面首次连接后收到后端状态快照。
- 后端停止后，前端连接状态离开已连接。
- 后端在同一地址重启后，前端自动重连并重新渲染状态快照。

![WebSocket 自动重连](screenshots/01-reconnected.png)

