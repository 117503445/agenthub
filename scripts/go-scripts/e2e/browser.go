package e2e

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

// browserSession 表示一个 Playwright 浏览器会话。
type browserSession struct {
	pw      *playwright.Playwright // pw 表示 Playwright 进程。
	browser playwright.Browser     // browser 表示当前浏览器实例。
	page    playwright.Page        // page 表示当前页面。
}

// newBrowserSession 使用 width 和 height 参数创建 Chromium 页面。
func newBrowserSession(width int, height int) (*browserSession, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("启动 Playwright 失败: %w", err)
	}
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		_ = pw.Stop()
		return nil, fmt.Errorf("启动 Chromium 失败: %w", err)
	}
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Viewport: &playwright.Size{Width: width, Height: height},
	})
	if err != nil {
		_ = browser.Close()
		_ = pw.Stop()
		return nil, fmt.Errorf("创建页面失败: %w", err)
	}
	return &browserSession{pw: pw, browser: browser, page: page}, nil
}

// Close 关闭当前浏览器会话。
func (s *browserSession) Close() {
	if s == nil {
		return
	}
	if s.browser != nil {
		_ = s.browser.Close()
	}
	if s.pw != nil {
		_ = s.pw.Stop()
	}
}

// gotoPage 使用 page 和 url 参数打开页面。
func gotoPage(page playwright.Page, url string) error {
	_, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	return err
}

// reloadPage 使用 page 参数刷新页面。
func reloadPage(page playwright.Page) error {
	_, err := page.Reload(playwright.PageReloadOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	return err
}

// screenshot 使用 page、path 和 fullPage 参数保存截图。
func screenshot(page playwright.Page, path string, fullPage bool) {
	_, _ = page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(path),
		FullPage: playwright.Bool(fullPage),
	})
}

// getByTestID 使用 page 和 id 参数返回 data-testid 定位器。
func getByTestID(page playwright.Page, id string) playwright.Locator {
	return page.GetByTestId(id)
}

// fillTestID 使用 page、id 和 value 参数填充 data-testid 对应输入框。
func fillTestID(page playwright.Page, id string, value string) error {
	return getByTestID(page, id).Fill(value)
}

// clickTestID 使用 page 和 id 参数点击 data-testid 对应元素。
func clickTestID(page playwright.Page, id string) error {
	return getByTestID(page, id).Click()
}

// selectTestID 使用 page、id 和 value 参数选择 data-testid 对应下拉框选项。
func selectTestID(page playwright.Page, id string, value string) error {
	_, err := getByTestID(page, id).SelectOption(playwright.SelectOptionValues{
		Values: &[]string{value},
	})
	return err
}

// expectTestIDValue 使用 page、id、expected 和 timeout 参数等待表单元素值等于 expected。
func expectTestIDValue(page playwright.Page, id string, expected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastValue string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := getByTestID(page, id).InputValue()
		if err == nil {
			lastValue = value
			if value == expected {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待元素 %q 值为 %q 超时，最后错误: %w", id, expected, lastErr)
	}
	return fmt.Errorf("等待元素 %q 值为 %q 超时，实际值: %s", id, expected, lastValue)
}

// expectTestIDDisabled 使用 page、id、expected 和 timeout 参数等待元素禁用状态符合预期。
func expectTestIDDisabled(page playwright.Page, id string, expected bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastDisabled bool
	var lastErr error
	for time.Now().Before(deadline) {
		disabled, err := getByTestID(page, id).IsDisabled()
		if err == nil {
			lastDisabled = disabled
			if disabled == expected {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待元素 %q 禁用状态为 %v 超时，最后错误: %w", id, expected, lastErr)
	}
	return fmt.Errorf("等待元素 %q 禁用状态为 %v 超时，实际状态: %v", id, expected, lastDisabled)
}

// expectTestIDAttributeAbsent 使用 page、id、name 和 timeout 参数等待元素属性不存在。
func expectTestIDAttributeAbsent(page playwright.Page, id string, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastHasAttribute bool
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := getByTestID(page, id).First().Evaluate(fmt.Sprintf("(element) => element.hasAttribute(%q)", name), nil)
		if err == nil {
			hasAttribute, ok := value.(bool)
			if ok {
				lastHasAttribute = hasAttribute
			}
			if ok && !hasAttribute {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待元素 %q 属性 %q 不存在超时，最后错误: %w", id, name, lastErr)
	}
	return fmt.Errorf("等待元素 %q 属性 %q 不存在超时，实际存在: %v", id, name, lastHasAttribute)
}

// expectTestIDPinnedToViewportBottom 使用 page、id 和 timeout 参数等待元素固定在视口底部。
func expectTestIDPinnedToViewportBottom(page playwright.Page, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := getByTestID(page, id).First().Evaluate(`(element) => {
			const style = getComputedStyle(element);
			const rect = element.getBoundingClientRect();
			const gap = Math.abs(window.innerHeight - rect.bottom);
			const bottom = Number.parseFloat(style.bottom);
			const positionMatched = style.position === 'sticky' || style.position === 'fixed';
			const bottomMatched = Number.isFinite(bottom) && Math.abs(bottom) <= 1;
			if (positionMatched && bottomMatched && gap <= 2) {
				return '';
			}
			return 'position=' + style.position +
				', bottom=' + style.bottom +
				', gap=' + gap.toFixed(2) +
				', rectBottom=' + rect.bottom.toFixed(2) +
				', viewport=' + window.innerHeight;
		}`, nil)
		if err == nil {
			if text, ok := value.(string); ok {
				lastState = text
				if text == "" {
					return nil
				}
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待元素 %q 固定在视口底部超时，最后错误: %w", id, lastErr)
	}
	return fmt.Errorf("等待元素 %q 固定在视口底部超时，最后状态: %s", id, lastState)
}

// expectLocatorBackgroundLuminance 使用 locator、minimum 和 timeout 参数等待背景亮度高于 minimum。
func expectLocatorBackgroundLuminance(locator playwright.Locator, minimum float64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastLuminance float64
	var lastText string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := locator.First().Evaluate("(element) => getComputedStyle(element).backgroundColor", nil)
		if err == nil {
			text, ok := value.(string)
			if ok {
				lastText = text
				lastLuminance = cssRGBLuminance(text)
				if lastLuminance >= minimum {
					return nil
				}
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待背景亮度不低于 %.1f 超时，最后错误: %w", minimum, lastErr)
	}
	return fmt.Errorf("等待背景亮度不低于 %.1f 超时，实际亮度 %.1f，颜色 %s", minimum, lastLuminance, lastText)
}

// cssRGBLuminance 使用 text 参数解析 CSS rgb/rgba 文本并返回感知亮度。
func cssRGBLuminance(text string) float64 {
	start := strings.Index(text, "(")
	end := strings.Index(text, ")")
	if start == -1 || end == -1 || end <= start {
		return 0
	}
	parts := strings.Split(text[start+1:end], ",")
	if len(parts) < 3 {
		return 0
	}
	values := make([]float64, 3)
	for index := range 3 {
		var value float64
		if _, err := fmt.Sscanf(strings.TrimSpace(parts[index]), "%f", &value); err != nil {
			return 0
		}
		values[index] = math.Max(0, math.Min(255, value))
	}
	return values[0]*0.299 + values[1]*0.587 + values[2]*0.114
}

// expectTestIDCount 使用 page、id 和 expected 参数等待元素数量符合预期。
func expectTestIDCount(page playwright.Page, id string, expected int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastCount int
	var lastErr error
	for time.Now().Before(deadline) {
		count, err := getByTestID(page, id).Count()
		if err == nil {
			lastCount = count
			if count == expected {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待元素 %q 数量为 %d 超时，最后错误: %w", id, expected, lastErr)
	}
	return fmt.Errorf("等待元素 %q 数量为 %d 超时，实际数量: %d", id, expected, lastCount)
}

// expectTestIDText 使用 page、id、expected 和 timeout 参数等待元素文本包含 expected。
func expectTestIDText(page playwright.Page, id string, expected string, timeout time.Duration) error {
	return expectLocatorText(getByTestID(page, id), expected, timeout)
}

// expectTestIDTextOrder 使用 page、id、before、after 和 timeout 参数等待文本顺序符合预期。
func expectTestIDTextOrder(page playwright.Page, id string, before string, after string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastText string
	var lastErr error
	for time.Now().Before(deadline) {
		text, err := getByTestID(page, id).TextContent()
		if err == nil {
			lastText = text
			beforeIndex := strings.Index(text, before)
			afterIndex := strings.Index(text, after)
			if beforeIndex >= 0 && afterIndex >= 0 && beforeIndex < afterIndex {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待文本 %q 位于 %q 前超时，最后错误: %w", before, after, lastErr)
	}
	return fmt.Errorf("等待文本 %q 位于 %q 前超时，实际文本: %s", before, after, lastText)
}

// expectToolCallSummaryText 使用 page、expected 和 timeout 参数等待工具调用标题包含 expected。
func expectToolCallSummaryText(page playwright.Page, expected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastText string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := getByTestID(page, "tool-call-details").First().Evaluate("(element) => element.querySelector('summary')?.textContent ?? ''", nil)
		if err == nil {
			if text, ok := value.(string); ok {
				lastText = text
				if strings.Contains(text, expected) {
					return nil
				}
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待工具调用标题包含 %q 超时，最后错误: %w", expected, lastErr)
	}
	return fmt.Errorf("等待工具调用标题包含 %q 超时，实际标题: %s", expected, lastText)
}

// expectTestIDNotText 使用 page、id、unexpected 和 timeout 参数等待元素文本不包含 unexpected。
func expectTestIDNotText(page playwright.Page, id string, unexpected string, timeout time.Duration) error {
	return expectLocatorNotText(getByTestID(page, id), unexpected, timeout)
}

// expectLocatorText 使用 locator、expected 和 timeout 参数等待元素文本包含 expected。
func expectLocatorText(locator playwright.Locator, expected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastText string
	var lastErr error
	for time.Now().Before(deadline) {
		text, err := locator.TextContent()
		if err == nil {
			lastText = text
			if strings.Contains(text, expected) {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待文本 %q 超时，最后错误: %w", expected, lastErr)
	}
	return fmt.Errorf("等待文本 %q 超时，实际文本: %s", expected, lastText)
}

// expectLocatorNotText 使用 locator、unexpected 和 timeout 参数等待元素文本不包含 unexpected。
func expectLocatorNotText(locator playwright.Locator, unexpected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastText string
	var lastErr error
	for time.Now().Before(deadline) {
		text, err := locator.TextContent()
		if err == nil {
			lastText = text
			if !strings.Contains(text, unexpected) {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待文本不包含 %q 超时，最后错误: %w", unexpected, lastErr)
	}
	return fmt.Errorf("等待文本不包含 %q 超时，实际文本: %s", unexpected, lastText)
}

// writeFile 使用 path 和 content 参数写入文本文件。
func writeFile(path string, content string) {
	_ = os.WriteFile(path, []byte(content), 0644)
}
