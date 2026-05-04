package e2e

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/117503445/agenthub/scripts/go-scripts/common"
	"github.com/playwright-community/playwright-go"
)

// runMockAgentGateCase 使用 ctx 参数运行 Mock Agent 开关 E2E 用例。
func runMockAgentGateCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeMockAgentGateReport(ctx.OutputDir, success, events)
	}()

	session, err := newBrowserSession(1280, 820)
	if err != nil {
		ctx.Logger.Errorf("创建浏览器失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("Mock Agent 开关 E2E 失败: %v", err)
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
	if err := expectSelectOptionAbsent(page, "agent-provider-select", "mock-claude-code", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectSelectOptionAbsent(page, "agent-provider-select", "mock-codex", 5*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("默认启动时，聊天框不展示 Mock Claude Code 和 Mock Codex。"))

	ctx.StopServer()
	mockBaseURL, mockStop, err := startMockAgentEnabledServer(ctx)
	if err != nil {
		return fail(err)
	}
	defer mockStop()
	if err := waitUntilReady(mockBaseURL); err != nil {
		return fail(fmt.Errorf("Mock Agent 服务未就绪: %w", err))
	}
	if err := gotoPage(page, mockBaseURL); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectSelectOptionPresent(page, "agent-provider-select", "mock-claude-code", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectSelectOptionPresent(page, "agent-provider-select", "mock-codex", 5*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("设置 MOCK_AGENT=1 后，聊天框展示 Mock Claude Code 和 Mock Codex。"))
	return true
}

// startMockAgentEnabledServer 使用 ctx 参数启动启用 MOCK_AGENT 的服务。
func startMockAgentEnabledServer(ctx E2EContext) (string, func(), error) {
	port, err := common.FindFreeTCPPort()
	if err != nil {
		return "", nil, err
	}
	legacyPort, err := common.FindFreeTCPPort()
	if err != nil {
		return "", nil, err
	}
	logsDir := filepath.Join(ctx.OutputDir, "mock-enabled-logs")
	dataDir := filepath.Join(ctx.OutputDir, "mock-enabled-data")
	_, stop, err := startServer(ctx.RootDir, ctx.ServerCmd, port, legacyPort, logsDir, dataDir, map[string]string{
		"MOCK_AGENT": "1",
	}, nil)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), stop, nil
}

// expectSelectOptionPresent 使用 page、testID、value 和 timeout 参数等待选择框包含指定选项。
func expectSelectOptionPresent(page playwright.Page, testID string, value string, timeout time.Duration) error {
	return expectSelectOptionState(page, testID, value, true, timeout)
}

// expectSelectOptionAbsent 使用 page、testID、value 和 timeout 参数等待选择框不包含指定选项。
func expectSelectOptionAbsent(page playwright.Page, testID string, value string, timeout time.Duration) error {
	return expectSelectOptionState(page, testID, value, false, timeout)
}

// expectSelectOptionState 使用 page、testID、value、present 和 timeout 参数等待选择框选项状态。
func expectSelectOptionState(page playwright.Page, testID string, value string, present bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		result, err := page.Evaluate(`([testID, value]) => {
			const select = document.querySelector('[data-testid="' + testID + '"]');
			if (!select) {
				return 'missing select';
			}
			const values = Array.from(select.options).map((option) => option.value);
			return values.includes(value) ? 'present' : 'absent';
		}`, []any{testID, value})
		if err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		state, ok := result.(string)
		if !ok {
			lastState = fmt.Sprintf("%v", result)
			time.Sleep(150 * time.Millisecond)
			continue
		}
		lastState = state
		if present && state == "present" {
			return nil
		}
		if !present && state == "absent" {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	expected := "present"
	if !present {
		expected = "absent"
	}
	if lastErr != nil {
		return fmt.Errorf("等待 %s 选项 %s=%s 超时，最后错误: %w", testID, value, expected, lastErr)
	}
	return fmt.Errorf("等待 %s 选项 %s=%s 超时，最后状态: %s", testID, value, expected, lastState)
}

// writeMockAgentGateReport 使用 outputDir、success 和 events 参数写入 Mock Agent 开关报告。
func writeMockAgentGateReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Mock Agent 开关 E2E 测试报告", success, events)
}
