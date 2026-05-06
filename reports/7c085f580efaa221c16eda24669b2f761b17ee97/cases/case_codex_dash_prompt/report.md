# Codex dash prompt E2E 测试报告

- 结果: 通过
- 日志: [test.log](logs/test.log)
- 服务日志: [server.log](logs/server.log)

## 过程

- Codex resume 后继续发送以 -- 开头的 prompt 时，会作为普通用户输入传递给 agent。

![Codex dash prompt](screenshots/01-codex-dash-prompt.png)

