package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// runManualStopNotificationCase 使用 ctx 参数运行手动停止通知 E2E 用例。
func runManualStopNotificationCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeManualStopNotificationReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("手动停止通知 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		events = append(events, reportImage("失败现场", "screenshots/failed.png"))
		return false
	}

	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return fail(err)
	}
	if err := installNotificationProbe(page); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-provider-select", "mock-claude-code"); err != nil {
		return fail(err)
	}

	prompt := "手动停止通知测试 " + strings.Repeat("保持流式输出，等待用户点击停止。", 16)
	if err := fillTestID(page, "message-input", prompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeValue(page, "send-button", "aria-label", "停止", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "send-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "已停止", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "send-button", 0, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectNotificationCountExactly(page, 0, 2*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-manual-stop-no-notification.png"), true)
	events = append(events, reportStep("用户手动停止 agent 输出后，聊天记录显示已停止，桌面通知数量保持为 0。"))
	events = append(events, reportImage("手动停止无通知", "screenshots/01-manual-stop-no-notification.png"))
	return true
}

// writeManualStopNotificationReport 使用 outputDir、success 和 events 参数写入手动停止通知报告。
func writeManualStopNotificationReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "手动停止通知 E2E 测试报告", success, events)
}
