# coding

这是一个 Go WebSocket 后端 + React 前端项目。

## 技术栈

- 后端：Go、`github.com/coder/websocket`、zerolog
- 前端：React、TypeScript、Vite、Tailwind CSS、Lucide React
- E2E：Go、playwright-go、Chromium
- 任务运行：go-task

## 常用命令

```bash
go-task run:web
```

运行后端服务，默认监听 `8080` 端口，可用 `AGENTHUB_PORT` 或 `--port` 修改。

```bash
go-task fe:dev
```

运行前端开发服务器，前端会把 `/ws` 和 `/healthz` 代理到 Go 后端。

```bash
go-task build:web
```

构建前端静态资源，并打包到 Go 后端二进制中。

```bash
go-task e2e -- --case case_ws
go-task e2e
```

运行单个或全部 E2E 测试。测试会自动启动构建后的后端服务，并验证 WebSocket 状态恢复、子路径加载和 agent 聊天流程。
每个 E2E 用例会独立启动一套后端服务，服务日志写入该用例目录，例如 `data/e2e/case_ws/logs/server.log`，用例结束后会关闭服务。

```bash
go-task test
```

运行 Go 单元测试、集成测试和全部 E2E 测试。
