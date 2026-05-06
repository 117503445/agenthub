package e2e

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/playwright-community/playwright-go"
)

// runMarkdownOrderedListCase 使用 ctx 参数运行 markdown 有序列表 E2E 用例。
func runMarkdownOrderedListCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeMarkdownOrderedListReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("Markdown 有序列表 E2E 失败: %v", err)
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

	prompt := "编号列表渲染测试：\n1. 创建阶段\n   检查代理开关。\n\n2. 代理阶段\n   写入代理地址。\n\n3. 路由阶段\n   解析数据面路径。\n\n4. 完成阶段\n   状态变为 READY。"
	if err := fillTestID(page, "message-input", prompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectMarkdownOrderedList(page, []string{"创建阶段", "代理阶段", "路由阶段", "完成阶段"}, 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "send-button", 0, 30*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-markdown-ordered-list.png"), true)
	events = append(events, reportStep("带续行和空行的 Markdown 有序列表会渲染为同一个有序列表，浏览器按 1 到 4 连续编号。"))
	events = append(events, reportImage("Markdown 有序列表", "screenshots/01-markdown-ordered-list.png"))
	return true
}

// expectMarkdownOrderedList 使用 page、expectedItems 和 timeout 参数等待有序列表连续渲染。
func expectMarkdownOrderedList(page playwright.Page, expectedItems []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`(expectedItems) => {
			const markdown = document.querySelector('[data-testid="assistant-markdown"]');
			if (!markdown) return 'missing markdown';
			const lists = Array.from(markdown.querySelectorAll('ol'));
			const matchingList = lists.find((list) => {
				const items = Array.from(list.querySelectorAll(':scope > li')).map((item) => item.textContent || '');
				return expectedItems.every((expected) => items.some((text) => text.includes(expected)));
			});
		if (!matchingList) {
			return 'ol=' + lists.length;
		}
		const directItems = Array.from(matchingList.querySelectorAll(':scope > li'));
		if (directItems.length !== expectedItems.length) {
			return 'li=' + directItems.length;
		}
			return '';
		}`, expectedItems)
		if err == nil {
			if text := fmt.Sprint(value); text == "" {
				return nil
			} else {
				lastState = text
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待 markdown 有序列表连续渲染超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待 markdown 有序列表连续渲染超时，最后状态: %s", lastState)
}

// writeMarkdownOrderedListReport 使用 outputDir、success 和 events 参数写入 Markdown 有序列表报告。
func writeMarkdownOrderedListReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Markdown 有序列表 E2E 测试报告", success, events)
}
