package e2e

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

// runMarkdownFilesystemLinksCase 使用 ctx 参数运行 markdown 文件链接 E2E 用例。
func runMarkdownFilesystemLinksCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeMarkdownFilesystemLinksReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("Markdown 文件链接 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		events = append(events, reportImage("失败现场", "screenshots/failed.png"))
		return false
	}

	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return fail(err)
	}
	if err := installClipboardProbe(page); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := selectTestID(page, "agent-provider-select", "mock-codex"); err != nil {
		return fail(err)
	}

	relativePath := "docs/需求.md"
	relativeAbsPath := filepath.Join(ctx.RootDir, relativePath)
	absolutePath := filepath.Join(ctx.RootDir, "README.md")
	relativeLinePath := "README.md:1"
	absoluteLinePath := absolutePath + ":1:2"
	prompt := fmt.Sprintf("请原样返回这些链接：[相对需求](%s) 和 [绝对说明](%s) 和 [相对行号](%s) 和 [绝对行号](%s)。", relativePath, absolutePath, relativeLinePath, absoluteLinePath)
	if err := fillTestID(page, "message-input", prompt); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "相对需求", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMarkdownLinkFilesystemHref(page, "相对需求", relativeAbsPath, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMarkdownLinkFilesystemHref(page, "绝对说明", absolutePath, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMarkdownLinkFilesystemHref(page, "相对行号", absolutePath, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMarkdownLinkFilesystemHref(page, "绝对行号", absolutePath, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMarkdownLinkFetchText(page, "相对需求", "# 需求", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMarkdownLinkFetchText(page, "绝对说明", "agenthub", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMarkdownLinkFetchText(page, "相对行号", "agenthub", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectMarkdownLinkFetchText(page, "绝对行号", "agenthub", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDCount(page, "send-button", 0, 30*time.Second); err != nil {
		return fail(err)
	}
	if err := clickTestID(page, "assistant-copy-button"); err != nil {
		return fail(err)
	}
	if err := expectClipboardText(page, "[相对需求](docs/需求.md)", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectClipboardText(page, fmt.Sprintf("[绝对说明](%s)", absolutePath), 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectClipboardText(page, "[相对行号](README.md:1)", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectClipboardText(page, fmt.Sprintf("[绝对行号](%s)", absoluteLinePath), 5*time.Second); err != nil {
		return fail(err)
	}
	if err := expectClipboardNotText(page, "/fs/content", 2*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-markdown-filesystem-links.png"), true)
	events = append(events, reportStep("Agent markdown 中的相对文件路径和绝对文件路径都会渲染为文件系统 API 链接，打开后返回后端文件内容。"))
	events = append(events, reportStep("复制 assistant 回复时保留 agent 返回的原始 markdown，不复制展示层改写后的 API 地址。"))
	events = append(events, reportImage("Markdown 文件链接", "screenshots/01-markdown-filesystem-links.png"))
	return true
}

// expectMarkdownLinkFilesystemHref 使用 page、label、expectedPath 和 timeout 参数等待 markdown 链接指向文件系统 API。
func expectMarkdownLinkFilesystemHref(page playwright.Page, label string, expectedPath string, timeout time.Duration) error {
	expectedPath = filepath.Clean(expectedPath)
	deadline := time.Now().Add(timeout)
	var lastHref string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`(label) => {
			const links = Array.from(document.querySelectorAll('[data-testid="assistant-markdown"] a'));
			const link = links.find((item) => item.textContent?.trim() === label);
			return link ? link.href : '';
		}`, label)
		if err == nil {
			lastHref = fmt.Sprint(value)
			if markdownFilesystemHrefMatches(lastHref, expectedPath) {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待 markdown 链接 %q 指向文件系统 API 超时，最后错误: %w", label, lastErr)
	}
	return fmt.Errorf("等待 markdown 链接 %q 指向 %q 超时，实际 href: %s", label, expectedPath, lastHref)
}

// markdownFilesystemHrefMatches 使用 href 和 expectedPath 参数判断链接是否匹配文件系统 API。
func markdownFilesystemHrefMatches(href string, expectedPath string) bool {
	parsed, err := url.Parse(href)
	if err != nil {
		return false
	}
	if !strings.HasSuffix(parsed.Path, "/fs/content") {
		return false
	}
	gotPath := parsed.Query().Get("path")
	return filepath.Clean(gotPath) == expectedPath
}

// expectMarkdownLinkFetchText 使用 page、label、expected 和 timeout 参数等待链接内容包含 expected。
func expectMarkdownLinkFetchText(page playwright.Page, label string, expected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastText string
	var lastErr error
	for time.Now().Before(deadline) {
		value, err := page.Evaluate(`async ([label]) => {
			const links = Array.from(document.querySelectorAll('[data-testid="assistant-markdown"] a'));
			const link = links.find((item) => item.textContent?.trim() === label);
			if (!link) {
				return 'missing link';
			}
			const response = await fetch(link.href);
			if (!response.ok) {
				return 'status=' + response.status;
			}
			return await response.text();
		}`, []any{label})
		if err == nil {
			lastText = fmt.Sprint(value)
			if strings.Contains(lastText, expected) {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待 markdown 链接 %q 内容包含 %q 超时，最后错误: %w", label, expected, lastErr)
	}
	return fmt.Errorf("等待 markdown 链接 %q 内容包含 %q 超时，实际内容: %s", label, expected, lastText)
}

// writeMarkdownFilesystemLinksReport 使用 outputDir、success 和 events 参数写入 markdown 文件链接报告。
func writeMarkdownFilesystemLinksReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Markdown 文件链接 E2E 测试报告", success, events)
}
