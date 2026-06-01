# 性能热路径 E2E 测试报告

- 结果: 通过
- 日志: [test.log](logs/test.log)
- 服务日志: [server.log](logs/server.log)

## 过程

- Codex delta 和完整文本事件合并后只显示一次，timeline 文件包含最终文本。
- 服务重启后，timeline 仍能恢复完整 assistant 文本。
