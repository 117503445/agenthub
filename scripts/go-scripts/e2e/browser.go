package e2e

import (
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

const testPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="

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

// installNotificationProbe 使用 page 参数注入桌面通知探针。
func installNotificationProbe(page playwright.Page) error {
	_, err := page.Evaluate(`() => {
		window.__agenthubNotifications = [];
		function FakeNotification(title, options) {
			window.__agenthubNotifications.push({
				title: String(title || ''),
				body: String(options?.body || ''),
			});
		}
		Object.defineProperty(FakeNotification, 'permission', { get: () => 'granted' });
		FakeNotification.requestPermission = () => Promise.resolve('granted');
		Object.defineProperty(window, 'Notification', {
			configurable: true,
			writable: true,
			value: FakeNotification,
		});
	}`, nil)
	return err
}

// attachTestImage 使用 page 和 name 参数通过文件选择控件添加测试图片。
func attachTestImage(page playwright.Page, name string) error {
	buffer, err := base64.StdEncoding.DecodeString(testPNGBase64)
	if err != nil {
		return err
	}
	return getByTestID(page, "image-file-input").SetInputFiles([]playwright.InputFile{{
		Name:     name,
		MimeType: "image/png",
		Buffer:   buffer,
	}})
}

// pasteTestImage 使用 page 和 name 参数模拟剪贴板粘贴图片。
func pasteTestImage(page playwright.Page, name string) error {
	_, err := page.Evaluate(`([name, base64]) => {
		const input = document.querySelector('[data-testid="message-input"]');
		if (!input) {
			throw new Error('missing message input');
		}
		const binary = atob(base64);
		const bytes = new Uint8Array(binary.length);
		for (let index = 0; index < binary.length; index += 1) {
			bytes[index] = binary.charCodeAt(index);
		}
		const file = new File([bytes], name, { type: 'image/png' });
		const data = new DataTransfer();
		data.items.add(file);
		const event = new ClipboardEvent('paste', {
			bubbles: true,
			cancelable: true,
			clipboardData: data,
		});
		input.dispatchEvent(event);
	}`, []any{name, testPNGBase64})
	return err
}

// expectImageAddButtonUsesPictureLogo 使用 page 和 timeout 参数等待添加图片按钮展示图片 logo。
func expectImageAddButtonUsesPictureLogo(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const button = document.querySelector('[data-testid="image-add-button"]');
			if (!button) {
				return 'missing button';
			}
			const logo = button.querySelector('img[data-testid="image-add-logo"]');
			const plusIcon = button.querySelector('svg.lucide-plus');
			if (!logo) {
				return 'missing image logo';
			}
			const rect = logo.getBoundingClientRect();
			const alt = logo.getAttribute('alt') ?? '';
			if (rect.width >= 14 && rect.height >= 14 && alt === '' && !plusIcon) {
				return '';
			}
			return 'width=' + rect.width.toFixed(2) +
				', height=' + rect.height.toFixed(2) +
				', alt=' + alt +
				', plus=' + Boolean(plusIcon);
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
		return fmt.Errorf("等待添加图片按钮展示图片 logo 超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待添加图片按钮展示图片 logo 超时，最后状态: %s", lastState)
}

// clickFirstTestID 使用 page 和 id 参数点击首个 data-testid 对应元素。
func clickFirstTestID(page playwright.Page, id string) error {
	return getByTestID(page, id).First().Click()
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

// expectSkillOptionSelected 使用 page、skillID 和 timeout 参数等待 skill 选项被键盘选中。
func expectSkillOptionSelected(page playwright.Page, skillID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastValue string
	var lastErr error
	selector := fmt.Sprintf(`[data-testid="skill-option"][data-skill-id="%s"]`, skillID)
	for time.Now().Before(deadline) {
		value, err := page.Locator(selector).GetAttribute("aria-selected")
		if err == nil {
			lastValue = value
			if value == "true" {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待 skill %q 选中超时，最后错误: %w", skillID, lastErr)
	}
	return fmt.Errorf("等待 skill %q 选中超时，实际 aria-selected: %s", skillID, lastValue)
}

// expectBrowserStorage 使用 page、expectedKeys、expectedValues 和 timeout 参数等待浏览器只持久化指定状态。
func expectBrowserStorage(
	page playwright.Page,
	expectedKeys []string,
	expectedValues map[string]string,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const localEntries = {};
			for (const key of Object.keys(window.localStorage).sort()) {
				localEntries[key] = window.localStorage.getItem(key) ?? '';
			}
			const sessionKeys = Object.keys(window.sessionStorage).sort();
			return {
				localKeys: Object.keys(localEntries).sort(),
				localEntries,
				sessionKeys,
			};
		}`, nil)
		if err == nil {
			state, ok := value.(map[string]any)
			if ok {
				lastState = fmt.Sprint(state)
				if browserStorageMatches(state, expectedKeys, expectedValues) {
					return nil
				}
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待浏览器持久化状态匹配超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待浏览器持久化状态匹配超时，最后状态: %s", lastState)
}

// browserStorageMatches 使用 state、expectedKeys 和 expectedValues 参数判断浏览器持久化状态是否匹配预期。
func browserStorageMatches(state map[string]any, expectedKeys []string, expectedValues map[string]string) bool {
	localKeys, ok := stringSliceFromAny(state["localKeys"])
	if !ok || !sameStringSlice(localKeys, expectedKeys) {
		return false
	}
	sessionKeys, ok := stringSliceFromAny(state["sessionKeys"])
	if !ok || len(sessionKeys) != 0 {
		return false
	}
	entries, ok := state["localEntries"].(map[string]any)
	if !ok {
		return false
	}
	for key, expected := range expectedValues {
		if fmt.Sprint(entries[key]) != expected {
			return false
		}
	}
	return true
}

// stringSliceFromAny 使用 value 参数转换 JavaScript 字符串数组。
func stringSliceFromAny(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, fmt.Sprint(item))
	}
	return result, true
}

// sameStringSlice 使用 actual 和 expected 参数比较字符串切片。
func sameStringSlice(actual []string, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index, item := range actual {
		if item != expected[index] {
			return false
		}
	}
	return true
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

// expectTestIDAttributeValue 使用 page、id、name、expected 和 timeout 参数等待至少一个元素属性符合预期。
func expectTestIDAttributeValue(page playwright.Page, id string, name string, expected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastValues string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`([id, name]) => {
			const elements = Array.from(document.querySelectorAll('[data-testid="' + id + '"]'));
			return elements.map((element) => element.getAttribute(name) ?? '');
		}`, []any{id, name})
		if err == nil {
			values, ok := value.([]any)
			if ok {
				parts := make([]string, 0, len(values))
				for _, item := range values {
					text := fmt.Sprint(item)
					parts = append(parts, text)
					if text == expected {
						return nil
					}
				}
				lastValues = strings.Join(parts, ", ")
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待元素 %q 属性 %q 为 %q 超时，最后错误: %w", id, name, expected, lastErr)
	}
	return fmt.Errorf("等待元素 %q 属性 %q 为 %q 超时，实际值: %s", id, name, expected, lastValues)
}

// expectTestIDNonEmpty 使用 page、id 和 timeout 参数等待元素文本非空。
func expectTestIDNonEmpty(page playwright.Page, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastText string
	var lastErr error
	for time.Now().Before(deadline) {
		text, err := getByTestID(page, id).TextContent()
		if err == nil {
			lastText = strings.TrimSpace(text)
			if lastText != "" {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待元素 %q 文本非空超时，最后错误: %w", id, lastErr)
	}
	return fmt.Errorf("等待元素 %q 文本非空超时，实际文本: %s", id, lastText)
}

// expectContextWindowMeter 使用 page 和 timeout 参数等待 context window 图标展示有效百分比。
func expectContextWindowMeter(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := getByTestID(page, "context-window-meter").First().Evaluate(`(element) => {
			const percent = Number.parseFloat(element.getAttribute('data-percent') || '');
			const label = element.textContent || '';
			if (Number.isFinite(percent) && percent >= 0 && percent <= 100 && label.includes('Context window')) {
				return '';
			}
			return 'percent=' + element.getAttribute('data-percent') + ', text=' + label;
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
		return fmt.Errorf("等待 context window 图标超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待 context window 图标超时，最后状态: %s", lastState)
}

// expectNotificationCount 使用 page、minimum 和 timeout 参数等待桌面通知数量达到下限。
func expectNotificationCount(page playwright.Page, minimum int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastCount int
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => window.__agenthubNotifications?.length ?? 0`, nil)
		if err == nil {
			switch typed := value.(type) {
			case int:
				lastCount = typed
			case float64:
				lastCount = int(typed)
			}
			if lastCount >= minimum {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待桌面通知数量达到 %d 超时，最后错误: %w", minimum, lastErr)
	}
	return fmt.Errorf("等待桌面通知数量达到 %d 超时，实际数量: %d", minimum, lastCount)
}

// expectTestIDDescendantText 使用 page、id、selector、expected 和 timeout 参数等待后代元素文本。
func expectTestIDDescendantText(page playwright.Page, id string, selector string, expected string, timeout time.Duration) error {
	return expectLocatorText(getByTestID(page, id).Locator(selector).First(), expected, timeout)
}

// expectTestIDsSameLine 使用 page、firstID、secondID、tolerance 和 timeout 参数等待两个元素同一行展示。
func expectTestIDsSameLine(page playwright.Page, firstID string, secondID string, tolerance float64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`([firstID, secondID, tolerance]) => {
			const find = (id) => document.querySelector('[data-testid="' + id + '"]');
			const first = find(firstID);
			const second = find(secondID);
			if (!first || !second) {
				return 'missing first=' + Boolean(first) + ', second=' + Boolean(second);
			}
			const firstRect = first.getBoundingClientRect();
			const secondRect = second.getBoundingClientRect();
			const firstCenter = firstRect.top + firstRect.height / 2;
			const secondCenter = secondRect.top + secondRect.height / 2;
			const diff = Math.abs(firstCenter - secondCenter);
			if (diff <= tolerance) {
				return '';
			}
			return 'center diff=' + diff.toFixed(2) + ', first=' + firstCenter.toFixed(2) + ', second=' + secondCenter.toFixed(2);
		}`, []any{firstID, secondID, tolerance})
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
		return fmt.Errorf("等待元素 %q 和 %q 同行超时，最后错误: %w", firstID, secondID, lastErr)
	}
	return fmt.Errorf("等待元素 %q 和 %q 同行超时，最后状态: %s", firstID, secondID, lastState)
}

// expectProjectMetaSingleCommit 使用 page 和 timeout 参数等待顶部 Git 哈希只出现一次。
func expectProjectMetaSingleCommit(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const meta = document.querySelector('[data-testid="project-meta"]');
			const commit = document.querySelector('[data-testid="project-commit-text"]');
			if (!meta || !commit) {
				return 'missing meta=' + Boolean(meta) + ', commit=' + Boolean(commit);
			}
			const commitText = (commit.textContent ?? '').trim();
			if (!commitText || commitText === '-') {
				return '';
			}
			const metaText = meta.textContent ?? '';
			const count = metaText.split(commitText).length - 1;
			if (count === 1) {
				return '';
			}
			return 'commit=' + commitText + ', count=' + count + ', text=' + metaText;
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
		return fmt.Errorf("等待顶部 Git 哈希只出现一次超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待顶部 Git 哈希只出现一次超时，最后状态: %s", lastState)
}

// expectChatTabCompact 使用 page 和 timeout 参数等待聊天标签页尺寸接近 paseo。
func expectChatTabCompact(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const tab = document.querySelector('[data-testid="chat-tab"]');
			if (!tab) {
				return 'missing tab';
			}
			const close = tab.querySelector('[data-testid="chat-tab-close-button"]');
			const rect = tab.getBoundingClientRect();
			const fontSize = Number.parseFloat(getComputedStyle(tab).fontSize);
			if (close && rect.height <= 34 && fontSize <= 13) {
				return '';
			}
			return 'height=' + rect.height.toFixed(2) + ', font=' + fontSize.toFixed(2) + ', close=' + Boolean(close);
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
		return fmt.Errorf("等待聊天标签页尺寸变紧凑超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待聊天标签页尺寸变紧凑超时，最后状态: %s", lastState)
}

// expectComposerShellLayout 使用 page 和 timeout 参数等待输入框居中且控件位于输入框内。
func expectComposerShellLayout(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const taskbar = document.querySelector('[data-testid="composer-taskbar"]');
			const shell = document.querySelector('[data-testid="composer-shell"]');
			if (!taskbar || !shell) {
				return 'missing taskbar=' + Boolean(taskbar) + ', shell=' + Boolean(shell);
			}
			const taskbarRect = taskbar.getBoundingClientRect();
			const shellRect = shell.getBoundingClientRect();
			const radius = Number.parseFloat(getComputedStyle(shell).borderTopLeftRadius);
			const selectCount = shell.querySelectorAll('select').length;
			const centered = Math.abs((shellRect.left + shellRect.right) / 2 - (taskbarRect.left + taskbarRect.right) / 2) <= 2;
			const widthOK = shellRect.width < taskbarRect.width * 0.9;
			if (centered && widthOK && radius >= 16 && selectCount >= 2) {
				return '';
			}
			return 'centered=' + centered +
				', width=' + shellRect.width.toFixed(2) +
				', taskbar=' + taskbarRect.width.toFixed(2) +
				', radius=' + radius.toFixed(2) +
				', selects=' + selectCount;
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
		return fmt.Errorf("等待输入框居中布局超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待输入框居中布局超时，最后状态: %s", lastState)
}

// expectComposerInitialSizing 使用 page 和 timeout 参数等待输入框初始高度、字号和背景符合预期。
func expectComposerInitialSizing(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const taskbar = document.querySelector('[data-testid="composer-taskbar"]');
			const shell = document.querySelector('[data-testid="composer-shell"]');
			const input = document.querySelector('[data-testid="message-input"]');
			if (!taskbar || !shell || !input) {
				return 'missing taskbar=' + Boolean(taskbar) + ', shell=' + Boolean(shell) + ', input=' + Boolean(input);
			}
			const inputRect = input.getBoundingClientRect();
			const fontSize = Number.parseFloat(getComputedStyle(input).fontSize);
			const taskbarLum = cssLuminance(getComputedStyle(taskbar).backgroundColor);
			const shellLum = cssLuminance(getComputedStyle(shell).backgroundColor);
			const compact = inputRect.height <= 72;
			const fontOK = Math.abs(fontSize - 16) <= 0.5;
			const shellGrey = shellLum < taskbarLum - 1;
			const taskbarWhite = taskbarLum >= 252;
			if (compact && fontOK && shellGrey && taskbarWhite) {
				return '';
			}
			return 'height=' + inputRect.height.toFixed(2) +
				', font=' + fontSize.toFixed(2) +
				', shellLum=' + shellLum.toFixed(1) +
				', taskbarLum=' + taskbarLum.toFixed(1);
			function cssLuminance(text) {
				const match = text.match(/rgba?\(([^)]+)\)/);
				if (!match) return 0;
				const parts = match[1].split(',').slice(0, 3).map((item) => Number.parseFloat(item.trim()));
				if (parts.length < 3 || parts.some((item) => !Number.isFinite(item))) return 0;
				return parts[0] * 0.299 + parts[1] * 0.587 + parts[2] * 0.114;
			}
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
		return fmt.Errorf("等待输入框初始尺寸和背景超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待输入框初始尺寸和背景超时，最后状态: %s", lastState)
}

// expectComposerExpandedSizing 使用 page 和 timeout 参数等待输入框随内容增高并保持上限。
func expectComposerExpandedSizing(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := getByTestID(page, "message-input").Evaluate(`(element) => {
			const rect = element.getBoundingClientRect();
			const style = getComputedStyle(element);
			const maxHeight = Number.parseFloat(style.maxHeight);
			if (rect.height >= 120 && rect.height <= 180 && Number.isFinite(maxHeight) && maxHeight <= 180) {
				return '';
			}
			return 'height=' + rect.height.toFixed(2) + ', maxHeight=' + style.maxHeight;
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
		return fmt.Errorf("等待输入框内容增高超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待输入框内容增高超时，最后状态: %s", lastState)
}

// expectComposerSelectWidthsFollowOptions 使用 page 和 timeout 参数等待输入框选择框宽度随实际选项变化。
func expectComposerSelectWidthsFollowOptions(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		if err := selectTestID(page, "agent-provider-select", "codex"); err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		shortProviderWidth, err := testIDWidth(page, "agent-provider-select")
		if err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		codexWidth, err := testIDWidth(page, "agent-model-select")
		if err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if err := selectTestID(page, "agent-provider-select", "mock-claude-code"); err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		longProviderWidth, err := testIDWidth(page, "agent-provider-select")
		if err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if err := selectTestID(page, "agent-provider-select", "mock-codex"); err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		mockCodexWidth, err := testIDWidth(page, "agent-model-select")
		if err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		textFitState, err := composerSelectTextFitState(page)
		if err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}
		providerFollowsSelection := longProviderWidth >= shortProviderWidth+24
		modelFollowsOptions := mockCodexWidth >= codexWidth+16
		if providerFollowsSelection && modelFollowsOptions && textFitState == "" {
			return nil
		}
		lastState = fmt.Sprintf(
			"shortProviderWidth=%.2f, longProviderWidth=%.2f, codexWidth=%.2f, mockCodexWidth=%.2f, textFit=%s",
			shortProviderWidth,
			longProviderWidth,
			codexWidth,
			mockCodexWidth,
			textFitState,
		)
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待输入框选择框宽度随选项变化超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待输入框选择框宽度随选项变化超时，最后状态: %s", lastState)
}

// composerSelectTextFitState 使用 page 参数返回选择框文本适配状态。
func composerSelectTextFitState(page playwright.Page) (string, error) {
	value, err := page.Evaluate(`() => {
		const ids = ['agent-provider-select', 'agent-model-select', 'agent-reasoning-select'];
		const context = document.createElement('canvas').getContext('2d');
		if (!context) {
			return 'missing canvas context';
		}
		const states = [];
		for (const id of ids) {
			const select = document.querySelector('[data-testid="' + id + '"]');
			if (!select) {
				continue;
			}
			const style = getComputedStyle(select);
			const rect = select.getBoundingClientRect();
			const option = select.options[select.selectedIndex];
			const text = option?.textContent?.trim() ?? '';
			const font = [
				style.fontStyle,
				style.fontVariant,
				style.fontWeight,
				style.fontSize + '/' + style.lineHeight,
				style.fontFamily,
			].join(' ');
			context.font = font;
			const textWidth = context.measureText(text).width;
			const paddingLeft = Number.parseFloat(style.paddingLeft) || 0;
			const paddingRight = Number.parseFloat(style.paddingRight) || 0;
			const available = rect.width - paddingLeft - paddingRight;
			const spare = available - textWidth;
			if (textWidth > available) {
				states.push(id + ': text=' + text + ', textWidth=' + textWidth.toFixed(2) + ', available=' + available.toFixed(2));
			} else if (spare > 18) {
				states.push(id + ': text=' + text + ', spare=' + spare.toFixed(2));
			}
		}
		return states.join('; ');
	}`, nil)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("选择框文本适配状态类型异常: %T", value)
	}
	return text, nil
}

// testIDWidth 使用 page 和 id 参数返回首个 data-testid 元素宽度。
func testIDWidth(page playwright.Page, id string) (float64, error) {
	value, err := getByTestID(page, id).First().Evaluate(`(element) => element.getBoundingClientRect().width`, nil)
	if err != nil {
		return 0, err
	}
	switch typed := value.(type) {
	case int:
		return float64(typed), nil
	case float64:
		return typed, nil
	default:
		return 0, fmt.Errorf("元素 %q 宽度类型异常: %T", id, value)
	}
}

// expectChatContentFontSize 使用 page 和 timeout 参数等待聊天正文使用 16px 字号。
func expectChatContentFontSize(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const elements = Array.from(document.querySelectorAll('.message-user > pre, .markdown-body'));
			if (!elements.length) {
				return 'missing message text';
			}
			const sizes = elements.map((element) => Number.parseFloat(getComputedStyle(element).fontSize));
			if (sizes.every((size) => Math.abs(size - 16) <= 0.5)) {
				return '';
			}
			return 'sizes=' + sizes.map((size) => size.toFixed(2)).join(',');
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
		return fmt.Errorf("等待聊天正文 16px 字号超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待聊天正文 16px 字号超时，最后状态: %s", lastState)
}

// expectMessageTimesIncludeSeconds 使用 page 和 timeout 参数等待消息时间精确到秒。
func expectMessageTimesIncludeSeconds(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const times = Array.from(document.querySelectorAll('[data-testid="message-time"]'))
				.map((element) => (element.textContent ?? '').trim())
				.filter(Boolean);
			if (!times.length) {
				return 'missing message time';
			}
			if (times.every((text) => /^\d{2}:\d{2}:\d{2}$/.test(text) || text === '已停止' || text === '失败')) {
				return '';
			}
			return 'times=' + times.join(',');
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
		return fmt.Errorf("等待消息时间精确到秒超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待消息时间精确到秒超时，最后状态: %s", lastState)
}

// expectMessageTimesOutsideBubbles 使用 page 和 timeout 参数等待消息时间位于消息内容外侧。
func expectMessageTimesOutsideBubbles(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const user = document.querySelector('.message-user');
			const assistant = document.querySelector('.message-assistant');
			const userTime = user?.querySelector('[data-testid="message-time"]');
			const assistantTime = assistant?.querySelector('[data-testid="message-time"]');
			if (!user || !assistant || !userTime || !assistantTime) {
				return 'missing user=' + Boolean(user) +
					', assistant=' + Boolean(assistant) +
					', userTime=' + Boolean(userTime) +
					', assistantTime=' + Boolean(assistantTime);
			}
			const userRect = user.getBoundingClientRect();
			const assistantRect = assistant.getBoundingClientRect();
			const userTimeRect = userTime.getBoundingClientRect();
			const assistantTimeRect = assistantTime.getBoundingClientRect();
			const userOutsideTop = userTimeRect.bottom <= userRect.top + 1;
			const userRight = Math.abs(userTimeRect.right - userRect.right) <= 2;
			const assistantOutsideTop = assistantTimeRect.bottom <= assistantRect.top + 1;
			const assistantLeft = Math.abs(assistantTimeRect.left - assistantRect.left) <= 10;
			if (userOutsideTop && userRight && assistantOutsideTop && assistantLeft) {
				return '';
			}
			return 'userOutsideTop=' + userOutsideTop +
				', userRight=' + userRight +
				', assistantOutsideTop=' + assistantOutsideTop +
				', assistantLeft=' + assistantLeft +
				', userTop=' + userRect.top.toFixed(2) +
				', userTimeBottom=' + userTimeRect.bottom.toFixed(2) +
				', userRightDelta=' + Math.abs(userTimeRect.right - userRect.right).toFixed(2) +
				', assistantTop=' + assistantRect.top.toFixed(2) +
				', assistantTimeBottom=' + assistantTimeRect.bottom.toFixed(2) +
				', assistantLeftDelta=' + Math.abs(assistantTimeRect.left - assistantRect.left).toFixed(2);
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
		return fmt.Errorf("等待消息时间位于消息外侧超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待消息时间位于消息外侧超时，最后状态: %s", lastState)
}

// expectNoMessageRoleLabels 使用 page 和 timeout 参数等待消息区域不显示角色名。
func expectNoMessageRoleLabels(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const legacyLabels = Array.from(document.querySelectorAll('.message-card > div:first-child > span:first-child'))
				.map((element) => (element.textContent ?? '').trim())
				.filter(Boolean);
			const explicitLabels = Array.from(document.querySelectorAll('[data-testid="message-role-label"]'))
				.map((element) => (element.textContent ?? '').trim())
				.filter(Boolean);
			const labels = [...legacyLabels, ...explicitLabels].filter((text) => text === '你' || text.includes('Codex') || text.includes('Claude'));
			if (!labels.length) {
				return '';
			}
			return 'labels=' + labels.join(',');
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
		return fmt.Errorf("等待消息区域隐藏角色名超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待消息区域隐藏角色名超时，最后状态: %s", lastState)
}

// expectUserCopyButtonOutsideMessage 使用 page 和 timeout 参数等待用户消息复制按钮位于消息框外侧右下方。
func expectUserCopyButtonOutsideMessage(page playwright.Page, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`() => {
			const message = document.querySelector('.message-user');
			const button = document.querySelector('[data-testid="user-copy-button"]');
			if (!message || !button) {
				return 'missing message=' + Boolean(message) + ', button=' + Boolean(button);
			}
			const messageRect = message.getBoundingClientRect();
			const buttonRect = button.getBoundingClientRect();
			const below = buttonRect.top >= messageRect.bottom - 1;
			const alignedRight = buttonRect.right >= messageRect.right - 1;
			if (below && alignedRight) {
				return '';
			}
			return 'messageBottom=' + messageRect.bottom.toFixed(2) +
				', messageRight=' + messageRect.right.toFixed(2) +
				', buttonTop=' + buttonRect.top.toFixed(2) +
				', buttonRight=' + buttonRect.right.toFixed(2);
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
		return fmt.Errorf("等待用户消息复制按钮位于外侧超时，最后错误: %w", lastErr)
	}
	return fmt.Errorf("等待用户消息复制按钮位于外侧超时，最后状态: %s", lastState)
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
