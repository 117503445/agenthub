package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
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
	title, err := page.Title()
	if err != nil {
		return fail(err)
	}
	if title != "agenthub" {
		return fail(fmt.Errorf("页面标题应为 agenthub，当前为 %q", title))
	}
	if err := installNotificationProbe(page); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "sidebar-identity", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "sidebar-identity", "AgentHub Projects", 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "sidebar-identity", "Projects", 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNonEmpty(page, "machine-name", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDsSameLine(page, "machine-name", "connection-state", 2, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "message-log", "创建聊天后开始", 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "project-add-button", 1, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "agent-settings-button", 1, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectLocatorBackgroundLuminance(page.Locator("aside"), 235, 2*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "前端使用 paseo 浅色工作台风格，侧栏左下角集中展示连接状态、机器名、添加项目和设置入口。")

	projectPath := ctx.RootDir
	projectDisplayName := filepath.Base(projectPath)
	if err := expectTestIDCount(page, "project-name-input", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "project-path-input", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-list", projectDisplayName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-name", projectDisplayName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "project-edit-button", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "project-delete-button", 1, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDsSameLine(page, "project-name", "project-delete-button", 2, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "project-list", projectPath, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-meta", projectPath, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-meta", "git", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDsSameLine(page, "project-path-text", "project-git-info", 2, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectProjectMetaSingleCommit(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "agent-config-panel", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "chat-tabs", "聊天 1", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "chat-tab-add-button", 1, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "chat-tab-close-button", 1, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectChatTabCompact(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDPinnedToViewportBottom(page, "composer-taskbar", 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectComposerShellLayout(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectComposerInitialSizing(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "context-window-meter", 1, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectContextWindowMeter(page, 5*time.Second); err != nil {
		return fail(err)
	}
	longComposerText := strings.Repeat("输入框自适应高度测试\n", 9)
	if err := fillTestID(page, "message-input", longComposerText); err != nil {
		return fail(err)
	}
	if err := expectComposerExpandedSizing(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", ""); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "message-log", "还没有消息", 2*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-chat-created.png"), true)
	steps = append(steps, "服务启动后自动添加 Git 工作目录为 project；顶部同一行展示完整路径和单份 git 信息，侧边栏只显示最后一级目录名，任务栏固定在底部。")

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

	if err := expectComposerSelectWidthsFollowOptions(page, 5*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "聊天框中的选择框宽度会根据当前实际选项内容调整。")

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
	if err := expectTestIDCount(page, "send-button", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "composer-agent-config", "Agent", 2*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "聊天框下方可以选择 agent、模型和推理级别，未输入时不显示发送按钮和 Agent 标签。")

	if err := fillTestID(page, "message-input", "/"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "skill-menu", "e2e-skill", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := page.Locator(`[data-testid="skill-option"][data-skill-id="e2e-skill"]`).Click(); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "message-input", "/e2e-skill ", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", ""); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", "/"); err != nil {
		return fail(err)
	}
	if err := expectSkillOptionSelected(page, "e2e-alpha", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("ArrowDown"); err != nil {
		return fail(err)
	}
	if err := expectSkillOptionSelected(page, "e2e-beta", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "message-input", "/e2e-beta ", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", ""); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", "/"); err != nil {
		return fail(err)
	}
	if err := expectSkillOptionSelected(page, "e2e-alpha", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Tab"); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "message-input", "/e2e-alpha ", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", ""); err != nil {
		return fail(err)
	}
	steps = append(steps, "输入 / 时可以选择后端返回的 skills，支持点击、键盘上下键选择和 Tab 快速确认当前项，并把选中的 skill 插入聊天输入框。")

	if err := expectImageAddButtonUsesPictureLogo(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := attachTestImage(page, "local-image.png"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "composer-attachments", "local-image.png", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := pasteTestImage(page, "pasted-image.png"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "composer-attachments", "pasted-image.png", 5*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "聊天框添加图片按钮使用图片 logo，可以选择本地图片，Ctrl+V 可以粘贴剪贴板图片，并在发送前展示附件预览。")

	firstPrompt := "第一条流式测试"
	if err := fillTestID(page, "message-input", firstPrompt); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "send-button", 1, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", firstPrompt, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "local-image.png", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "pasted-image.png", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMessageTimesIncludeSeconds(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectNoMessageRoleLabels(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeValue(page, "chat-status-dot", "data-status", "running", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeValue(page, "project-status-dot", "data-status", "running", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "chat-tabs", firstPrompt, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "chat-tabs", "聊天 1", 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "Mock Codex", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDDescendantText(page, "message-log", `[data-testid="assistant-markdown"] h2`, "Mock Codex", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMessageTimesOutsideBubbles(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectChatContentFontSize(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectToolCallSummaryText(page, "pwd", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDTextOrder(page, "message-log", "pwd", "正在回复", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeAbsent(page, "tool-call-details", "open", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "user-copy-button", 1, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "assistant-copy-button", 1, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectUserCopyButtonOutsideMessage(page, 5*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "02-streaming.png"), true)
	steps = append(steps, "Mock Codex 通过内置 Codex CLI 请求后端 OpenAI mock 模型服务，工具调用标题直接展示命令并排在输出前。")

	if err := expectTestIDCount(page, "send-button", 0, 30*time.Second); err != nil {
		return fail(err)
	}
	if err := expectNotificationCount(page, 1, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeValue(page, "chat-status-dot", "data-status", "success", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeValue(page, "project-status-dot", "data-status", "success", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "message-log"); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "chat-status-dot", 0, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "project-status-dot", 0, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDDisabled(page, "agent-provider-select", true, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDDisabled(page, "agent-model-select", false, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDDisabled(page, "agent-reasoning-select", false, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "composer-agent-config", "已锁定", 2*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "聊天页开始会话后只锁定 agent，模型和推理级别仍然可调整，聊天框不显示已锁定文字。")

	firstDraft := "第一个聊天页未发送草稿"
	secondDraft := "第二个聊天页未发送草稿"
	if err := fillTestID(page, "message-input", firstDraft); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "chat-tab-add-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "chat-tabs", "聊天 2", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "message-input", "", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", secondDraft); err != nil {
		return fail(err)
	}
	if err := page.Locator(`[data-testid="chat-tab"]`, playwright.PageLocatorOptions{HasText: firstPrompt}).Click(); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "message-input", firstDraft, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := page.Locator(`[data-testid="chat-tab"]`, playwright.PageLocatorOptions{HasText: "聊天 2"}).Click(); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "message-input", secondDraft, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "agent-provider-select", "mock-codex", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "agent-model-select", "mock-codex-gpt-5.5", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "agent-reasoning-select", "xhigh", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-provider-select", "mock-claude-code"); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "agent-model-select", "mock-claude-sonnet", 10*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "新聊天默认继承上一次选择的 agent、模型和推理级别；每个聊天 Tab 保留独立输入草稿；E2E 使用 Mock Claude Code 命令连接服务端 mock 模型服务。")

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
	if err := expectTestIDText(page, "chat-tabs", secondPrompt, 10*time.Second); err != nil {
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
	steps = append(steps, "聊天页第一次发送 prompt 后，Tab 标题显示本次聊天主题；agent 正在输出时直接输入并回车，会停止上一轮并发送新的 prompt。")

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
	if err := expectTestIDText(page, "chat-tabs", secondPrompt, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "Mock Claude", 20*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "03-restored.png"), true)
	steps = append(steps, "刷新页面后仍能从后端恢复 project、聊天和 mock Claude 正在输出的会话。")

	if err := expectTestIDCount(page, "send-button", 0, 30*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "agent 输出完成后，停止按钮重新变回发送按钮。")

	if err := clickTestID(page, "chat-tab-add-button"); err != nil {
		return fail(err)
	}
	if err := runMockPlanFlow(page, "mock-codex", "Mock Codex Plan", "Mock Codex 执行结果"); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "chat-tab-add-button"); err != nil {
		return fail(err)
	}
	if err := runMockPlanFlow(page, "mock-claude-code", "Mock Claude Plan", "Mock Claude 执行结果"); err != nil {
		return fail(err)
	}
	steps = append(steps, "Mock Codex 和 Mock Claude Code 都能在 mock 模型服务下生成 plan、按用户意见修订，并点击开始执行。")

	if err := clickTestID(page, "chat-tab-add-button"); err != nil {
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
	if err := expectTestIDAttributeValue(page, "chat-status-dot", "data-status", "error", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeValue(page, "project-status-dot", "data-status", "error", 10*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "agent 报错时，错误信息会返回并显示在前端聊天记录中。")
	return true
}

// runMockPlanFlow 使用 page、provider、planTitle 和 executionTitle 参数验证 mock plan 全流程。
func runMockPlanFlow(page playwright.Page, provider string, planTitle string, executionTitle string) error {
	if err := selectTestID(page, "agent-provider-select", provider); err != nil {
		return err
	}
	if err := clickTestID(page, "plan-mode-toggle"); err != nil {
		return err
	}
	if err := expectTestIDAttributeValue(page, "plan-mode-toggle", "data-active", "true", 5*time.Second); err != nil {
		return err
	}
	planPrompt := fmt.Sprintf("生成 %s plan 模式测试", provider)
	if err := fillTestID(page, "message-input", planPrompt); err != nil {
		return err
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return err
	}
	if err := expectTestIDText(page, "plan-card", "待确认", 20*time.Second); err != nil {
		return err
	}
	if err := expectTestIDText(page, "plan-card", planTitle, 20*time.Second); err != nil {
		return err
	}
	planReply := "请把第一条改成先写测试"
	if err := fillTestID(page, "message-input", planReply); err != nil {
		return err
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return err
	}
	if err := expectTestIDText(page, "message-log", planReply, 10*time.Second); err != nil {
		return err
	}
	if err := expectTestIDText(page, "plan-card", "先写测试", 20*time.Second); err != nil {
		return err
	}
	if err := clickTestID(page, "plan-execute-button"); err != nil {
		return err
	}
	if err := expectTestIDText(page, "message-log", "开始执行已确认的 plan", 10*time.Second); err != nil {
		return err
	}
	if err := expectTestIDText(page, "message-log", executionTitle, 20*time.Second); err != nil {
		return err
	}
	if err := expectTestIDCount(page, "send-button", 0, 30*time.Second); err != nil {
		return err
	}
	return nil
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
