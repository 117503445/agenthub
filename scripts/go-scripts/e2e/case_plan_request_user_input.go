package e2e

import (
	"fmt"
	"path/filepath"
	"time"
)

// runPlanRequestUserInputCase 使用 ctx 参数验证 Codex plan 模式 request_user_input 交互。
func runPlanRequestUserInputCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "Plan request_user_input E2E 测试报告", success, events)
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
		ctx.Logger.Errorf("Plan request_user_input E2E 失败: %v", err)
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
	if err := clickTestID(page, "plan-mode-toggle"); err != nil {
		return fail(err)
	}
	if err := expectTestIDAttributeValue(page, "plan-mode-toggle", "data-active", "true", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", "生成 request_user_input plan 模式测试"); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "request-user-input-card", "Proceed with the plan?", 30*time.Second); err != nil {
		return fail(err)
	}
	option := page.Locator(`[data-testid="request-user-input-option"][data-question-id="confirm_path"][data-option-label="Yes (Recommended)"]`)
	if err := option.Click(); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "request-user-input-submit"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "request-user-input-card", "已提交", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "plan-card", "Mock Codex Plan", 30*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "plan-card", "待确认", 10*time.Second); err != nil {
		return fail(err)
	}

	events = append(events, reportStep("Codex plan 模式下，agent 通过 request_user_input 请求确认，前端提交答案后继续生成 plan。"))
	return true
}
