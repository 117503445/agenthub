package e2e

import (
	"fmt"
	"path/filepath"
	"time"
)

const e2eAgentHubToken = "e2e-agenthub-token"

// runTokenAuthCase 使用 ctx 参数运行 AgentHub Token 鉴权 E2E 用例。
func runTokenAuthCase(ctx E2EContext) (success bool) {
	subpathURL := ctx.BaseURL + "/console/#/"
	ctx.Logger.Infof("打开页面: %s", subpathURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeTokenAuthReport(ctx.OutputDir, success, events)
	}()

	if _, err := osStat(filepath.Join(ctx.LogsDir, "server.log")); err != nil {
		ctx.Logger.Errorf("未找到当前用例服务日志: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}

	session, err := newBrowserSession(1366, 900)
	if err != nil {
		ctx.Logger.Errorf("创建浏览器失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("Token 鉴权 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		events = append(events, reportImage("失败现场", "screenshots/failed.png"))
		return false
	}

	if err := gotoPage(page, subpathURL); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "token-auth-form", 1, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectBrowserStorage(page, []string{}, map[string]string{}, 5*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-token-required.png"), true)
	events = append(events, reportStep("AGENTHUB_TOKEN 非空时，前端先展示 token 输入界面，不连接工作台。"))
	events = append(events, reportImage("需要 Token", "screenshots/01-token-required.png"))

	if err := fillTestID(page, "agenthub-token-input", "wrong-token"); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "agenthub-token-submit"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "token-auth-error", "Token 不正确", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectBrowserStorage(page, []string{}, map[string]string{}, 5*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("输入错误 token 时，前端保持在 token 输入界面，且不会持久化错误 token。"))

	if err := fillTestID(page, "agenthub-token-input", e2eAgentHubToken); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "agenthub-token-submit"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-list", filepath.Base(ctx.RootDir), 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectBrowserStorage(page, []string{"AGENTHUB_TOKEN"}, map[string]string{
		"AGENTHUB_TOKEN": e2eAgentHubToken,
	}, 5*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "02-authenticated.png"), true)
	events = append(events, reportStep("输入正确 token 后，前端连接 WebSocket 并只把 AGENTHUB_TOKEN 写入持久化状态。"))
	events = append(events, reportImage("鉴权通过", "screenshots/02-authenticated.png"))

	if err := reloadPage(page); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "token-auth-form", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectBrowserStorage(page, []string{"AGENTHUB_TOKEN"}, map[string]string{
		"AGENTHUB_TOKEN": e2eAgentHubToken,
	}, 5*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("刷新页面后，前端复用已储存的 AGENTHUB_TOKEN，不再储存其它状态。"))
	return true
}

// writeTokenAuthReport 使用 outputDir、success 和 events 参数写入 Token 鉴权测试报告。
func writeTokenAuthReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Token 鉴权 E2E 测试报告", success, events)
}
