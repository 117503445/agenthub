# AgentHub

AgentHub 是一个用于在浏览器中管理本地代码项目和 AI Agent 会话的 Web 工作台。后端运行在本机，前端内嵌在 Go 服务中，通过 WebSocket 同步状态，并可启动 Codex、Claude Code 或已配置的 Agent Profile 来处理项目内的聊天任务。

## 安装

安装最新 master Release 中适配当前系统的二进制：

```bash
curl -fsSL https://raw.githubusercontent.com/117503445/agenthub/master/install.sh | sh
```

默认安装到 `/usr/local/bin/agenthub`。如需安装到用户目录：

```bash
curl -fsSL https://raw.githubusercontent.com/117503445/agenthub/master/install.sh | AGENTHUB_INSTALL_DIR="$HOME/.local/bin" sh
```

## 启动

```bash
agenthub
```

服务默认监听 `17375` 端口。启动后在浏览器打开：

```text
http://127.0.0.1:17375
```

可以通过环境变量或 CLI 参数修改端口：

```bash
AGENTHUB_PORT=18080 agenthub
agenthub --port 18080
```

如需开启访问 token：

```bash
AGENTHUB_TOKEN="your-token" agenthub
agenthub --token "your-token"
```

如需指定数据目录：

```bash
AGENTHUB_DATA="$HOME/.agenthub/data" agenthub
```

## 使用

1. 打开 Web 页面后，选择或新增 Project。
2. 在 Project 中新建聊天页，选择 Agent Profile、模型和思考深度。
3. 输入 Prompt 后，AgentHub 会启动对应 agent 并流式展示返回结果。
4. Agent 输出中可以继续输入新 Prompt，也可以点击停止按钮中断当前输出。
5. 在设置页中可以管理 Agent Profile、模型列表和 Profile 环境变量。
