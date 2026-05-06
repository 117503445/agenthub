package e2e

import (
	"fmt"
	"path/filepath"
	"time"
)

// runContextWindowMeterCase 使用 ctx 参数运行 context window 图标显示规则 E2E 用例。
func runContextWindowMeterCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeContextWindowMeterReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("Context window 图标 E2E 失败: %v", err)
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
	if err := expectTestIDCount(page, "context-window-meter", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("没有 agent 真实上报 context window 数据时，不显示 context window 图标。"))

	if err := selectTestID(page, "agent-provider-select", "mock-codex"); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", "低 context window 用量测试"); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "send-button", 0, 30*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "context-window-meter", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-context-window-hidden.png"), true)
	events = append(events, reportStep("agent 真实上报数据低于 context window 的 1/4 时，也不显示图标。"))
	events = append(events, reportImage("Context window hidden", "screenshots/01-context-window-hidden.png"))
	return true
}

// writeContextWindowMeterReport 使用 outputDir、success 和 events 参数写入 context window 图标报告。
func writeContextWindowMeterReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Context window 图标 E2E 测试报告", success, events)
}
