package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// runAgentChatCase 使用 ctx 参数运行 Agent 聊天 E2E 用例。
func runAgentChatCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	steps := make([]string, 0)
	defer func() {
		writeAgentChatReport(ctx.OutputDir, success, steps)
	}()

	if _, err := osStat(filepath.Join(ctx.LogsDir, "server.log")); err != nil {
		ctx.Logger.Errorf("未找到当前用例服务日志: %v", err)
		steps = append(steps, fmt.Sprintf("用例失败: %v", err))
		return false
	}

	session, err := newBrowserSession(1440, 920)
	if err != nil {
		ctx.Logger.Errorf("创建浏览器失败: %v", err)
		steps = append(steps, fmt.Sprintf("用例失败: %v", err))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("Agent 聊天 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		steps = append(steps, fmt.Sprintf("用例失败: %v", err))
		return false
	}

	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}

	projectName := fmt.Sprintf("E2E Project %d", time.Now().Unix())
	projectPath := ctx.RootDir
	if err := fillTestID(page, "project-name-input", projectName); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "project-path-input", projectPath); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "project-save-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-list", projectName, 10*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "创建一个绑定本机目录的 project，并在列表中看到该 project。")

	if err := clickTestID(page, "chat-new-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "chat-tabs", "聊天 1", 10*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-chat-created.png"), true)
	steps = append(steps, "选中 project 后可以创建聊天页。")

	firstPrompt := "第一条流式测试"
	if err := fillTestID(page, "message-input", firstPrompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "send-button", "停止", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", firstPrompt, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "Mock Claude", 20*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "02-streaming.png"), true)
	steps = append(steps, "首次输入 prompt 后后端启动 agent，并把 mock Claude 输出流式返回前端。")

	secondPrompt := "第二条长流式测试"
	if err := fillTestID(page, "message-input", secondPrompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", secondPrompt, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "send-button", "停止", 10*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "agent 正在输出时直接输入并回车，会停止上一轮并发送新的 prompt。")

	if err := reloadPage(page); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-list", projectName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", secondPrompt, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "Mock Claude", 20*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "03-restored.png"), true)
	steps = append(steps, "刷新页面后仍能从后端恢复 project、聊天和正在输出的会话。")

	if err := expectTestIDText(page, "send-button", "发送", 30*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "agent 输出完成后，停止按钮重新变回发送按钮。")
	return true
}

// writeAgentChatReport 使用 outputDir、success 和 steps 参数写入 Agent 聊天报告。
func writeAgentChatReport(outputDir string, success bool, steps []string) {
	report := []string{
		"# Agent 聊天 E2E 测试报告",
		"",
		fmt.Sprintf("- 结果: %s", passText(success)),
		"- 日志: [test.log](logs/test.log)",
		"- 服务日志: [server.log](logs/server.log)",
		"",
		"## 步骤",
		"",
	}
	for _, step := range steps {
		report = append(report, "- "+step)
	}
	report = append(report,
		"",
		"## 截图",
		"",
		"![项目和聊天](screenshots/01-chat-created.png)",
		"",
		"![流式输出](screenshots/02-streaming.png)",
		"",
		"![刷新恢复](screenshots/03-restored.png)",
	)
	if !success {
		report = append(report, "", "![失败现场](screenshots/failed.png)")
	}
	writeFile(filepath.Join(outputDir, "report.md"), strings.Join(report, "\n")+"\n")
}
