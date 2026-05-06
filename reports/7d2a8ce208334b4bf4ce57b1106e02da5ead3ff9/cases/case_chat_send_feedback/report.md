# 聊天发送反馈 E2E 测试报告

- 结果: 通过
- 日志: [test.log](logs/test.log)
- 服务日志: [server.log](logs/server.log)

## 过程

- 发送请求失败时，聊天框展示服务端错误，发送按钮从转圈状态恢复为发送。
- 发送成功后，聊天消息区立即滚动到底部。

![聊天发送反馈](screenshots/01-chat-send-feedback.png)

