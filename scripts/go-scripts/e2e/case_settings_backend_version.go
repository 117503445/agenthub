package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

// runSettingsBackendVersionCase 使用 ctx 参数运行设置页后端版本信息 E2E 用例。
func runSettingsBackendVersionCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeSettingsBackendVersionReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("设置页后端版本信息 E2E 失败: %v", err)
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
	if err := clickTestID(page, "agent-settings-button"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "agent-settings-backend-info", "后端信息", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectBackendVersionVisible(page, 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectBackendBuildTimeVisible(page, 5*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-backend-version.png"), true)
	events = append(events, reportStep("进入设置页后，可以看到后端返回的版本和构建时间。"))
	events = append(events, reportImage("后端版本和构建时间", "screenshots/01-backend-version.png"))
	return true
}

// expectBackendVersionVisible 使用 page 和 timeout 参数等待后端版本文本不是空值和占位符。
func expectBackendVersionVisible(page playwright.Page, timeout time.Duration) error {
	return expectSettingsInfoVisible(page, "backend-version-text", "后端版本信息", timeout)
}

// expectBackendBuildTimeVisible 使用 page 和 timeout 参数等待后端构建时间文本不是空值和占位符。
func expectBackendBuildTimeVisible(page playwright.Page, timeout time.Duration) error {
	return expectSettingsInfoVisible(page, "backend-build-time-text", "后端构建时间", timeout)
}

// expectSettingsInfoVisible 使用 page、testID、label 和 timeout 参数等待设置页信息值可见。
func expectSettingsInfoVisible(page playwright.Page, testID string, label string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastText := ""
	var lastErr error
	for time.Now().Before(deadline) {
		text, err := getByTestID(page, testID).TextContent()
		if err == nil {
			lastText = strings.TrimSpace(text)
			if lastText != "" && lastText != "-" {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待%s超时，最后错误: %w", label, lastErr)
	}
	return fmt.Errorf("等待%s超时，实际文本: %s", label, lastText)
}

// writeSettingsBackendVersionReport 使用 outputDir、success 和 events 参数写入设置页后端版本信息报告。
func writeSettingsBackendVersionReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "设置页后端版本信息 E2E 测试报告", success, events)
}
