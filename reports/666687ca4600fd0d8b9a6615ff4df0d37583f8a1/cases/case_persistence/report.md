# 后端 JSON 持久化 E2E 测试报告

- 结果: 通过
- 日志: [test.log](logs/test.log)
- 服务日志: [server.log](logs/server.log)

## 步骤

- 通过 WebSocket 创建 project，服务端返回 project.changed。
- 写操作完成后，AGENTHUB_DATA 下立即出现 state.json，并包含新增 project。
- 原服务存活时，同一个 AGENTHUB_DATA 的第二个进程会自行退出。
- 服务重启后从 state.json 恢复 project，并在首个状态快照中返回。
