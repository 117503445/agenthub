package e2e

import (
	"fmt"
	"path/filepath"
	"time"
)

// runProjectIconCase 使用 ctx 参数运行项目图标 E2E 用例。
func runProjectIconCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeProjectIconReport(ctx.OutputDir, success, events)
	}()

	session, err := newBrowserSession(1280, 800)
	if err != nil {
		ctx.Logger.Errorf("创建浏览器失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("项目图标 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		events = append(events, reportImage("失败现场", "screenshots/failed.png"))
		return false
	}

	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return fail(err)
	}
	if err := expectAgentHubIcon(page, 5*time.Second); err != nil {
		return fail(err)
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-project-icon.png"), true)
	events = append(events, reportStep("页面声明浅色 Tabler chart-bubble SVG 图标，浏览器可以通过 favicon 链接加载。"))
	events = append(events, reportImage("项目图标已加载", "screenshots/01-project-icon.png"))
	return true
}

// writeProjectIconReport 使用 outputDir、success 和 events 参数写入项目图标测试报告。
func writeProjectIconReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "项目图标 E2E 测试报告", success, events)
}
