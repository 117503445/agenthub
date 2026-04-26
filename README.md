# coding

这是一个 Go WebSocket 后端 + React 前端项目。

## 技术栈

- 后端：Go、`github.com/coder/websocket`、zerolog
- 前端：React、TypeScript、Vite、Tailwind CSS、Lucide React
- E2E：Python、uv、Playwright
- 任务运行：go-task

## 常用命令

```bash
task run:web
```

运行后端服务，默认监听 `8080` 端口。

```bash
task fe:dev
```

运行前端开发服务器，前端会把 `/ws` 和 `/healthz` 代理到 Go 后端。

```bash
task build:web
```

构建前端静态资源，并打包到 Go 后端二进制中。

```bash
task e2e:install
task e2e:run
```

安装并运行 E2E 测试。测试会自动启动构建后的后端服务，并验证 WebSocket 连接和消息回环。
