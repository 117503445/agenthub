package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// runWSCase 使用 ctx 参数运行 WebSocket 和子路径状态同步用例。
func runWSCase(ctx E2EContext) (success bool) {
	subpathURL := ctx.BaseURL + "/console/#/"
	ctx.Logger.Infof("打开页面: %s", subpathURL)
	steps := make([]string, 0)
	defer func() {
		writeWSReport(ctx.OutputDir, success, steps)
	}()

	if _, err := osStat(filepath.Join(ctx.LogsDir, "server.log")); err != nil {
		ctx.Logger.Errorf("未找到当前用例服务日志: %v", err)
		steps = append(steps, fmt.Sprintf("用例失败: %v", err))
		return false
	}

	session, err := newBrowserSession(1366, 900)
	if err != nil {
		ctx.Logger.Errorf("创建浏览器失败: %v", err)
		steps = append(steps, fmt.Sprintf("用例失败: %v", err))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("WebSocket 状态同步 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		steps = append(steps, fmt.Sprintf("用例失败: %v", err))
		return false
	}

	if err := gotoPage(page, subpathURL); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-connected.png"), true)
	steps = append(steps, "页面从 /console/ 子路径打开后，WebSocket 状态变为已连接，并收到后端状态快照。")

	projectName := filepath.Base(ctx.RootDir)
	if err := expectTestIDCount(page, "project-name-input", 0, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "project-path-input", ctx.RootDir); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "project-save-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-list", projectName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "project-list", ctx.RootDir, 2*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "chat-tabs", "聊天 1", 10*time.Second); err != nil {
		return fail(err)
	}
	hashValue, err := page.Evaluate("window.location.hash")
	if err != nil {
		return fail(err)
	}
	hashText, _ := hashValue.(string)
	if !strings.HasPrefix(hashText, "#/projects/") {
		return fail(fmt.Errorf("hash 路由未指向 project: %s", hashText))
	}
	if err := reloadPage(page); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-list", projectName, 10*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "02-restored.png"), true)
	steps = append(steps, "创建 project 后自动打开聊天页，hash 路由指向 project，刷新页面仍从后端内存状态恢复。")
	return true
}

// writeWSReport 使用 outputDir、success 和 steps 参数写入 WebSocket 测试报告。
func writeWSReport(outputDir string, success bool, steps []string) {
	report := []string{
		"# WebSocket 和子路径状态同步 E2E 测试报告",
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
		"![连接成功](screenshots/01-connected.png)",
		"",
		"![状态恢复](screenshots/02-restored.png)",
	)
	if !success {
		report = append(report, "", "![失败现场](screenshots/failed.png)")
	}
	writeFile(filepath.Join(outputDir, "report.md"), strings.Join(report, "\n")+"\n")
}
