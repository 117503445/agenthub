package e2e

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/117503445/agenthub/internal/wsapp"
)

// runCodexAppServerAlignmentCase 使用 ctx 参数验证 Codex app-server 关键体验对齐。
func runCodexAppServerAlignmentCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "Codex app-server 对齐 E2E 测试报告", success, events)
	}()

	session, err := newBrowserSession(1440, 920)
	if err != nil {
		ctx.Logger.Errorf("创建浏览器失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("Codex app-server 对齐 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		events = append(events, reportImage("失败现场", "screenshots/failed.png"))
		return false
	}

	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-provider-select", "mock-codex"); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", "MOCK_CODEX_CAPABILITY_BOOTSTRAP"); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "Capability discovery complete", 30*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "composer-taskbar", "mock-codex-dynamic", 10*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("Mock Codex app-server 启动后，AgentHub 从 model/list 更新模型和思考深度。"))

	if err := fillTestID(page, "message-input", "/"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "skill-menu", "e2e-codex-skill", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", "/e2e-codex-skill 结构化参数"); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "结构化 skill e2e-codex-skill 参数：结构化参数", 30*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("命中 Codex skill 的 slash command 会以结构化 skill 输入发送给 app-server。"))

	chatID, err := firstMockCodexChatID(ctx.BaseURL)
	if err != nil {
		return fail(err)
	}
	if err := sendCodexOutputSchemaPrompt(ctx.BaseURL, chatID); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "Output schema received", 30*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("WebSocket chat.send 可携带 outputSchema，并透传到 Codex turn/start。"))

	ctx.StopServer()
	restartStop, err := restartServerAtSameBaseURL(ctx, "codex-alignment-restart-logs")
	if err != nil {
		return fail(err)
	}
	defer restartStop()
	if err := waitUntilReady(ctx.BaseURL); err != nil {
		return fail(fmt.Errorf("重启服务未就绪: %w", err))
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "Output schema received", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", "MOCK_CODEX_HISTORY_HYDRATE"); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "History hydrate complete", 30*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-codex-app-server-alignment.png"), true)
	events = append(events, reportStep("服务重启后，前端通过聊天 timeline 恢复历史消息并继续当前 Codex turn。"))
	events = append(events, reportImage("Codex app-server 对齐", "screenshots/01-codex-app-server-alignment.png"))
	return true
}

// firstMockCodexChatID 使用 baseURL 参数返回当前快照里的首个 Mock Codex 聊天页。
func firstMockCodexChatID(baseURL string) (string, error) {
	conn, err := dialPersistenceWS(baseURL)
	if err != nil {
		return "", err
	}
	defer conn.CloseNow()
	message, err := readPersistenceMessage(conn, "state.snapshot", 5*time.Second)
	if err != nil {
		return "", err
	}
	var snapshot wsapp.Snapshot
	if err := json.Unmarshal(message.Payload, &snapshot); err != nil {
		return "", err
	}
	for _, chat := range snapshot.Chats {
		if chat.AgentProvider == wsapp.AgentProviderMockCodex {
			return chat.ID, nil
		}
	}
	return "", fmt.Errorf("未找到 Mock Codex 聊天页")
}

// sendCodexOutputSchemaPrompt 使用 baseURL 和 chatID 参数发送携带 outputSchema 的聊天请求。
func sendCodexOutputSchemaPrompt(baseURL string, chatID string) error {
	conn, err := dialPersistenceWS(baseURL)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	if _, err := readPersistenceMessage(conn, "state.snapshot", 5*time.Second); err != nil {
		return err
	}
	payload := map[string]any{
		"chatId":   chatID,
		"prompt":   "MOCK_CODEX_OUTPUT_SCHEMA",
		"planMode": false,
		"outputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
			"required": []string{"message"},
		},
	}
	return sendE2EClientMessage(conn, "chat.send", payload)
}
