package e2e

import (
	"fmt"
	"path/filepath"
	"time"
)

// runCodexTimelineEventsCase 使用 ctx 参数验证 Codex app-server 事件会按顺序展示为 timeline 片段。
func runCodexTimelineEventsCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "Codex timeline 事件 E2E 测试报告", success, events)
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
		ctx.Logger.Errorf("Codex timeline 事件 E2E 失败: %v", err)
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
	if err := fillTestID(page, "message-input", "MOCK_CODEX_TIMELINE_EVENTS"); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	for _, expected := range []string{
		"command_approval",
		"file_approval",
		"Reasoning",
		"terminal",
		"apply_patch",
		"file_change",
		"sub_agent",
		"Timeline events complete",
	} {
		if err := expectTestIDText(page, "message-log", expected, 30*time.Second); err != nil {
			return fail(err)
		}
	}
	if err := expectTestIDText(page, "context-window-meter", "128K", 10*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-codex-timeline-events.png"), true)
	events = append(events, reportStep("Codex command、approval、terminal、patch、file change、usage 事件都展示到聊天 timeline。"))
	events = append(events, reportImage("Codex timeline 事件", "screenshots/01-codex-timeline-events.png"))
	return true
}
