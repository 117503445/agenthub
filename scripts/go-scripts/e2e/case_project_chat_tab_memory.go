package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/playwright-community/playwright-go"
)

// runProjectChatTabMemoryCase 使用 ctx 参数运行 Project 聊天页记忆 E2E 用例。
func runProjectChatTabMemoryCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeProjectChatTabMemoryReport(ctx.OutputDir, success, events)
	}()

	session, err := newBrowserSession(1366, 900)
	if err != nil {
		ctx.Logger.Errorf("创建浏览器失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("Project 聊天页记忆 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		events = append(events, reportImage("失败现场", "screenshots/failed.png"))
		return false
	}

	secondProjectPath := filepath.Join(ctx.OutputDir, "remembered-project")
	if err := os.MkdirAll(secondProjectPath, 0755); err != nil {
		return fail(err)
	}

	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}

	firstProjectName := filepath.Base(ctx.RootDir)
	secondProjectName := filepath.Base(secondProjectPath)
	if err := expectTestIDText(page, "project-list", firstProjectName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectActiveChatTabText(page, "聊天 1", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "chat-tab-add-button"); err != nil {
		return fail(err)
	}
	if err := expectActiveChatTabText(page, "聊天 2", 10*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("第一个 Project 新建并选中第二个聊天页。"))

	if err := clickTestID(page, "project-add-button"); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "project-path-input", secondProjectPath); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "project-save-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-list", secondProjectName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-meta", secondProjectPath, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectActiveChatTabText(page, "聊天 1", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "chat-tab-add-button"); err != nil {
		return fail(err)
	}
	if err := expectActiveChatTabText(page, "聊天 2", 10*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-second-project-chat.png"), true)
	events = append(events, reportStep("第二个 Project 也新建并选中第二个聊天页。"))
	events = append(events, reportImage("第二个 Project 的聊天页", "screenshots/01-second-project-chat.png"))

	if err := page.Locator(`[data-testid="project-name"]`, playwright.PageLocatorOptions{HasText: firstProjectName}).Click(); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-meta", ctx.RootDir, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectActiveChatTabText(page, "聊天 2", 10*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("切回第一个 Project 时，自动恢复到此前选中的第二个聊天页。"))

	if err := page.Locator(`[data-testid="project-name"]`, playwright.PageLocatorOptions{HasText: secondProjectName}).Click(); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-meta", secondProjectPath, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectActiveChatTabText(page, "聊天 2", 10*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "02-restored-tabs.png"), true)
	events = append(events, reportStep("再次切回第二个 Project 时，同样恢复该 Project 自己的第二个聊天页。"))
	events = append(events, reportImage("恢复聊天页", "screenshots/02-restored-tabs.png"))
	return true
}

// writeProjectChatTabMemoryReport 使用 outputDir、success 和 events 参数写入测试报告。
func writeProjectChatTabMemoryReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Project 聊天页记忆 E2E 测试报告", success, events)
}
