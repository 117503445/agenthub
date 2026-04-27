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
	if err := expectLocatorBackgroundLuminance(page.Locator("aside"), 235, 2*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "前端使用 paseo 浅色工作台风格，侧栏保持低对比浅灰应用壳。")

	projectPath := ctx.RootDir
	projectDisplayName := filepath.Base(projectPath)
	if err := expectTestIDCount(page, "project-name-input", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "project-path-input", projectPath); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "project-save-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-list", projectDisplayName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "project-list", projectPath, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "chat-tabs", "聊天 1", 10*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-chat-created.png"), true)
	steps = append(steps, "创建 project 只需要输入目录，侧边栏只显示最后一级目录名，并自动打开第一个聊天页。")

	if err := clickTestID(page, "agent-settings-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "agent-settings-page", "Claude Code", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "agent-model-id-input", "claude-custom-test"); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "agent-model-label-input", "Claude Custom Test"); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "agent-model-add-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "agent-settings-model-list", "Claude Custom Test", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "back-to-chat-button"); err != nil {
		return fail(err)
	}
	steps = append(steps, "Agent 设置页可以向 Claude Code 模型选项列表添加新模型。")

	if err := expectTestIDText(page, "agent-config-panel", "Mock Claude Code", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-provider-select", "claude-code"); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-model-select", "claude-custom-test"); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-provider-select", "mock-codex"); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "agent-model-select", "mock-codex-gpt-5.5", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "agent-reasoning-select", "xhigh", 10*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "聊天页可以选择 Mock Codex gpt-5.5 和推理级别。")

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
	if err := expectTestIDText(page, "message-log", "工具调用", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "exec_command", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeAbsent(page, "tool-call-details", "open", 5*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "02-streaming.png"), true)
	steps = append(steps, "首次输入 prompt 后后端启动 Mock Codex，流式返回文本并默认折叠工具调用结果。")

	if err := expectTestIDText(page, "send-button", "发送", 30*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDDisabled(page, "agent-provider-select", true, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDDisabled(page, "agent-model-select", true, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDDisabled(page, "agent-reasoning-select", true, 5*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "聊天页开始会话后，agent、模型和推理级别被锁定。")

	if err := clickTestID(page, "chat-new-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "chat-tabs", "聊天 2", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-provider-select", "mock-claude-code"); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "agent-model-select", "mock-claude-sonnet", 10*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "E2E 使用 Mock Claude Code 命令连接服务端 mock 模型服务。")

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
	thirdPrompt := "第三条打断测试"
	if err := fillTestID(page, "message-input", thirdPrompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", thirdPrompt, 10*time.Second); err != nil {
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
	if err := expectTestIDText(page, "project-list", projectDisplayName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", thirdPrompt, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "Mock Claude", 20*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "03-restored.png"), true)
	steps = append(steps, "刷新页面后仍能从后端恢复 project、聊天和 mock Claude 正在输出的会话。")

	if err := expectTestIDText(page, "send-button", "发送", 30*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "agent 输出完成后，停止按钮重新变回发送按钮。")

	if err := clickTestID(page, "chat-new-button"); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-provider-select", "mock-codex"); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", "MOCK_AGENT_ERROR"); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "mock codex error", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "失败", 10*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "agent 报错时，错误信息会返回并显示在前端聊天记录中。")
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
