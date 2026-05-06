package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// runChatSendFeedbackCase 使用 ctx 参数运行聊天发送反馈 E2E 用例。
func runChatSendFeedbackCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeChatSendFeedbackReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("聊天发送反馈 E2E 失败: %v", err)
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

	imageNames := make([]string, 0, 9)
	for index := 1; index <= 9; index += 1 {
		imageNames = append(imageNames, fmt.Sprintf("too-many-%02d.png", index))
	}
	if err := attachTestImages(page, imageNames); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "composer-attachments", "too-many-09.png", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", "发送失败反馈测试"); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectSendButtonLoading(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "composer-error", "图片最多支持 8 张", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeValue(page, "send-button", "aria-label", "发送", 5*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("发送请求失败时，聊天框展示服务端错误，发送按钮从转圈状态恢复为发送。"))

	if err := clickTestID(page, "chat-tab-add-button"); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-provider-select", "mock-codex"); err != nil {
		return fail(err)
	}
	if err := page.SetViewportSize(1440, 520); err != nil {
		return fail(err)
	}
	longPrompt := "发送成功滚动测试\n" + strings.Repeat("成功后应立即滚动到底部。\n", 80)
	if err := fillTestID(page, "message-input", longPrompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectSendButtonLoading(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "发送成功滚动测试", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMessageLogScrolledToBottom(page, 8, 10*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-chat-send-feedback.png"), true)
	events = append(events, reportStep("发送成功后，聊天消息区立即滚动到底部。"))
	events = append(events, reportImage("聊天发送反馈", "screenshots/01-chat-send-feedback.png"))
	return true
}

// writeChatSendFeedbackReport 使用 outputDir、success 和 events 参数写入聊天发送反馈报告。
func writeChatSendFeedbackReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "聊天发送反馈 E2E 测试报告", success, events)
}
