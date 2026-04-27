package e2e

import (
	"fmt"
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

// expectTestIDText 使用 page、id、expected 和 timeout 参数等待元素文本包含 expected。
func expectTestIDText(page playwright.Page, id string, expected string, timeout time.Duration) error {
	return expectLocatorText(getByTestID(page, id), expected, timeout)
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

// writeFile 使用 path 和 content 参数写入文本文件。
func writeFile(path string, content string) {
	_ = os.WriteFile(path, []byte(content), 0644)
}
