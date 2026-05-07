package e2e

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/playwright-community/playwright-go"
)

// runAgentProviderEmptyProfileCase 使用 ctx 参数验证空 agentProfile 不会生成空白 Agent 选项。
func runAgentProviderEmptyProfileCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeAgentProviderEmptyProfileReport(ctx.OutputDir, success, events)
	}()

	session, err := newBrowserSession(1280, 820)
	if err != nil {
		ctx.Logger.Errorf("Agent Provider 空 Profile E2E 失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("Agent Provider 空 Profile E2E 失败: %v", err)
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
	if err := expectAgentProviderOptionsWithoutEmpty(page, 5*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("默认启动时，未开始聊天页的空 agentProfile 不会在 Agent 下拉框中追加空白选项。"))
	return true
}

// expectAgentProviderOptionsWithoutEmpty 使用 page 和 timeout 参数等待 Agent 下拉框只有真实选项。
func expectAgentProviderOptionsWithoutEmpty(page playwright.Page, timeout time.Duration) error {
	return expectPageState(page, "等待 Agent 下拉框去除空白选项", `() => {
			const select = document.querySelector('[data-testid="agent-provider-select"]');
			if (!select) {
				return 'missing select';
			}
			const options = Array.from(select.options).map((option) => ({
				value: option.value,
				label: option.textContent?.trim() ?? '',
			}));
			if (options.some((option) => option.value === '' || option.label === '')) {
				return '包含空选项: ' + JSON.stringify(options);
			}
			const values = options.map((option) => option.value).join(',');
			if (values !== 'claude-code,codex') {
				return '选项值不正确: ' + JSON.stringify(options);
			}
			const labels = options.map((option) => option.label).join(',');
			if (labels !== 'Claude Code,Codex') {
				return '选项文案不正确: ' + JSON.stringify(options);
			}
			return '';
		}`, nil, timeout)
}

// writeAgentProviderEmptyProfileReport 使用 outputDir、success 和 events 参数写入测试报告。
func writeAgentProviderEmptyProfileReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Agent Provider 空 Profile E2E 测试报告", success, events)
}
