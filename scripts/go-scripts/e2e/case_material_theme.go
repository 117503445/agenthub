package e2e

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/playwright-community/playwright-go"
)

// runMaterialThemeCase 使用 ctx 参数运行 Material 主题前端重构 E2E 用例。
func runMaterialThemeCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeMaterialThemeReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("Material 主题前端重构 E2E 失败: %v", err)
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
	if err := expectMaterialThemeContract(page, 5*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-material-theme.png"), true)
	events = append(events, reportStep("前端根节点已切换为 Material 主题，侧栏、聊天标签和输入区使用浅色分层表面与统一主色 token。"))
	events = append(events, reportImage("Material 主题界面", "screenshots/01-material-theme.png"))
	return true
}

// expectMaterialThemeContract 使用 page 和 timeout 参数等待 Material 主题契约生效。
func expectMaterialThemeContract(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastState := ""
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(materialThemeContractScript())
		if err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		state, _ := value.(string)
		lastState = state
		if state == "ok" {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待 Material 主题契约超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待 Material 主题契约超时，最后状态: %s", lastState)
}

// materialThemeContractScript 返回浏览器端 Material 主题契约检查脚本。
func materialThemeContractScript() string {
	return `() => {
		const root = document.querySelector('main[data-theme="material"].theme-material');
		if (!root) return 'missing material root';
		const rootStyle = getComputedStyle(root);
		if (rootStyle.getPropertyValue('--agenthub-primary').trim().toLowerCase() !== '#0b57d0') return 'primary token mismatch';
		if (rootStyle.getPropertyValue('--agenthub-secondary').trim().toLowerCase() !== '#d3e3fd') return 'secondary token mismatch';
		const sidebar = document.querySelector('[data-testid="sidebar"]');
		if (!sidebar) return 'missing sidebar';
		if (relativeLuminance(getComputedStyle(sidebar).backgroundColor) < 0.9) return 'sidebar is too dark';
		const tabs = document.querySelector('[data-testid="chat-tabs"]');
		if (!tabs) return 'missing chat tabs';
		if (relativeLuminance(getComputedStyle(tabs).backgroundColor) < 0.9) return 'tabs are too dark';
		const composer = document.querySelector('[data-testid="composer-shell"]');
		if (!composer) return 'missing composer shell';
		const composerStyle = getComputedStyle(composer);
		if (relativeLuminance(composerStyle.backgroundColor) < 0.92) return 'composer is too dark';
		if (!composerStyle.boxShadow || composerStyle.boxShadow === 'none') return 'composer has no elevation';
		return 'ok';

		function relativeLuminance(color) {
			const channels = color.match(/\d+(\.\d+)?/g)?.slice(0, 3).map(Number) ?? [0, 0, 0];
			const [r, g, b] = channels.map((value) => {
				const c = value / 255;
				return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
			});
			return 0.2126 * r + 0.7152 * g + 0.0722 * b;
		}
	}`
}

// writeMaterialThemeReport 使用 outputDir、success 和 events 参数写入 Material 主题测试报告。
func writeMaterialThemeReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Material 主题前端重构 E2E 测试报告", success, events)
}
