package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runNoContextWindowCase 使用 ctx 参数验证前后端不再保留 context window 展示状态。
func runNoContextWindowCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeNoContextWindowReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("Context window 移除 E2E 失败: %v", err)
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
	if err := assertStateFileOmitsContextWindow(ctx.DataDir); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-no-context-window.png"), true)
	events = append(events, reportStep("聊天页不显示 context window，后端状态也不再输出 contextWindow 字段。"))
	events = append(events, reportImage("Context window removed", "screenshots/01-no-context-window.png"))
	return true
}

// assertStateFileOmitsContextWindow 使用 dataDir 参数验证状态文件不包含 contextWindow 字段。
func assertStateFileOmitsContextWindow(dataDir string) error {
	statePath := filepath.Join(dataDir, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), `"contextWindow"`) {
		return fmt.Errorf("状态文件仍包含 contextWindow 字段: %s", statePath)
	}
	return nil
}

// writeNoContextWindowReport 使用 outputDir、success 和 events 参数写入 context window 移除报告。
func writeNoContextWindowReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Context window 移除 E2E 测试报告", success, events)
}
