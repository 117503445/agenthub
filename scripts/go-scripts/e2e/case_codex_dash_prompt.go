package e2e

import (
	"fmt"
	"path/filepath"
	"time"
)

// runCodexDashPromptCase 使用 ctx 参数运行 Codex dash prompt E2E 用例。
func runCodexDashPromptCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeCodexDashPromptReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("Codex dash prompt E2E 失败: %v", err)
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
	if err := fillTestID(page, "message-input", "建立 Codex resume 会话"); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "建立 Codex resume 会话", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "send-button", 0, 30*time.Second); err != nil {
		return fail(err)
	}
	if err := expectFileText(filepath.Join(ctx.DataDir, "state.json"), "mock-codex-app-thread", 10*time.Second); err != nil {
		return fail(err)
	}

	prompt := "--tag latest 是默认值吗"
	if err := fillTestID(page, "message-input", prompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", prompt, 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "message-log", "unexpected argument", 2*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-codex-dash-prompt.png"), true)
	events = append(events, reportStep("Codex resume 后继续发送以 -- 开头的 prompt 时，会作为普通用户输入传递给 agent。"))
	events = append(events, reportImage("Codex dash prompt", "screenshots/01-codex-dash-prompt.png"))
	return true
}

// writeCodexDashPromptReport 使用 outputDir、success 和 events 参数写入 Codex dash prompt 报告。
func writeCodexDashPromptReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Codex dash prompt E2E 测试报告", success, events)
}
