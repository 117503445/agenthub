# Agent Profile 管理 E2E 测试报告

- 结果: 通过
- 日志: [test.log](logs/test.log)
- 服务日志: [server.log](logs/server.log)

## 过程

- 首次初始化数据目录时，状态快照返回真实和 Mock 内置 Profile，并展示后端启动环境变量。
- 删除过的内置 Profile 可以通过新增内置 Profile 再次添加。
- Effective Env 使用后端启动环境变量叠加 Profile 环境变量，支持覆盖和 unset，并返回完整密钥值。
- Profile 和模型项支持增删改查。
- 设置页展示 Profile 列表、后端启动环境变量和完整 Effective Env，并支持切换默认模型。
