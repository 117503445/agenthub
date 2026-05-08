package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/117503445/agenthub/internal/wsapp"
)

// runCodexResumeContextCase 使用 ctx 参数验证 Codex thread 恢复和 context window 展示。
func runCodexResumeContextCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "Codex 恢复和上下文 E2E 测试报告", success, events)
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
		ctx.Logger.Errorf("Codex 恢复和上下文 E2E 失败: %v", err)
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
	firstPrompt := "建立 Codex context window 会话"
	if err := fillTestID(page, "message-input", firstPrompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", firstPrompt, 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "context-window-meter", "上下文", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "context-window-meter", "128K", 5*time.Second); err != nil {
		return fail(err)
	}
	sessionID, err := waitCodexResumeSessionID(ctx.DataDir, 10*time.Second)
	if err != nil {
		return fail(err)
	}
	events = append(events, reportStep("首轮 Codex 运行后，前端展示 context window，后端持久化 thread ID。"))

	ctx.StopServer()
	restartStop, err := restartServerAtSameBaseURL(ctx, "resume-context-restart-logs")
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
	resumePrompt := "MOCK_CODEX_RESUME_CONTEXT"
	if err := fillTestID(page, "message-input", resumePrompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	expected := "已恢复 Codex thread " + sessionID
	if err := expectTestIDText(page, "message-log", expected, 30*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "context-window-meter", "128K", 5*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-codex-resume-context.png"), true)
	events = append(events, reportStep("后端重启后，AgentHub 使用持久化 thread ID 恢复 Codex app-server 会话并继续发送 prompt。"))
	events = append(events, reportImage("Codex 恢复和上下文", "screenshots/01-codex-resume-context.png"))
	return true
}

// waitCodexResumeSessionID 使用 dataDir 和 timeout 参数等待 Codex thread ID 写入 state.json。
func waitCodexResumeSessionID(dataDir string, timeout time.Duration) (string, error) {
	var sessionID string
	err := waitForCondition("等待 Codex thread ID 持久化", timeout, func() (bool, string, error) {
		id, err := readCodexResumeSessionID(dataDir)
		if err != nil {
			return false, "", err
		}
		sessionID = id
		return id != "", "sessionID 为空", nil
	})
	return sessionID, err
}

// readCodexResumeSessionID 使用 dataDir 参数读取首个 Mock Codex 聊天页的 thread ID。
func readCodexResumeSessionID(dataDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	if err != nil {
		return "", err
	}
	var state struct {
		Chats []wsapp.Chat `json:"chats"` // Chats 表示聊天页摘要。
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", err
	}
	for _, chat := range state.Chats {
		if chat.AgentProvider == wsapp.AgentProviderMockCodex && chat.AgentPersistence != nil && strings.TrimSpace(chat.AgentPersistence.SessionID) != "" {
			return chat.AgentPersistence.SessionID, nil
		}
	}
	return "", nil
}
