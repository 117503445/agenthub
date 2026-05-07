package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/playwright-community/playwright-go"
)

// runProjectReorderCase 使用 ctx 参数运行 Project 排序 E2E 用例。
func runProjectReorderCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeProjectReorderReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("Project 排序 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		events = append(events, reportImage("失败现场", "screenshots/failed.png"))
		return false
	}

	firstProjectPath := filepath.Join(ctx.OutputDir, "alpha-project")
	secondProjectPath := filepath.Join(ctx.OutputDir, "beta-project")
	for _, projectPath := range []string{firstProjectPath, secondProjectPath} {
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			return fail(fmt.Errorf("创建测试 project 目录失败: %w", err))
		}
	}

	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}

	rootName := filepath.Base(ctx.RootDir)
	firstName := filepath.Base(firstProjectPath)
	secondName := filepath.Base(secondProjectPath)
	if err := createProjectFromSidebar(page, firstProjectPath); err != nil {
		return fail(err)
	}
	if err := expectProjectOrder(page, []string{firstName, rootName}, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := createProjectFromSidebar(page, secondProjectPath); err != nil {
		return fail(err)
	}
	if err := expectProjectOrder(page, []string{secondName, firstName, rootName}, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := dragProjectBefore(page, rootName, secondName); err != nil {
		return fail(err)
	}
	if err := expectProjectOrder(page, []string{rootName, secondName, firstName}, 10*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-project-reorder.png"), true)
	events = append(events, reportStep("新增 Project 默认插入列表顶部，拖拽 Project 后列表顺序按拖拽结果更新。"))
	events = append(events, reportImage("Project 拖拽排序", "screenshots/01-project-reorder.png"))
	return true
}

// createProjectFromSidebar 使用 page 和 projectPath 参数从侧栏创建 Project。
func createProjectFromSidebar(page playwright.Page, projectPath string) error {
	if err := clickTestID(page, "project-add-button"); err != nil {
		return err
	}
	if err := fillTestID(page, "project-path-input", projectPath); err != nil {
		return err
	}
	return clickTestID(page, "project-save-button")
}

// expectProjectOrder 使用 page、expectedNames 和 timeout 参数等待 Project 列表顺序匹配。
func expectProjectOrder(page playwright.Page, expectedNames []string, timeout time.Duration) error {
	return waitForCondition(fmt.Sprintf("等待 Project 顺序 %v", expectedNames), timeout, func() (bool, string, error) {
		value, err := page.Evaluate(`() => Array.from(document.querySelectorAll('[data-testid="project-name"]')).map((item) => item.textContent?.trim() || '')`, nil)
		if err != nil {
			return false, "", err
		}
		names, ok := value.([]any)
		if !ok {
			return false, fmt.Sprintf("顺序类型异常: %T", value), nil
		}
		got := make([]string, 0, len(names))
		for _, item := range names {
			got = append(got, fmt.Sprint(item))
		}
		return projectOrderHasPrefix(got, expectedNames), fmt.Sprintf("实际顺序: %v", got), nil
	})
}

// projectOrderHasPrefix 使用 got 和 expected 参数判断 Project 顺序前缀是否匹配。
func projectOrderHasPrefix(got []string, expected []string) bool {
	if len(got) < len(expected) {
		return false
	}
	for index, expectedName := range expected {
		if got[index] != expectedName {
			return false
		}
	}
	return true
}

// dragProjectBefore 使用 page、sourceName 和 targetName 参数把 source 拖到 target 前方。
func dragProjectBefore(page playwright.Page, sourceName string, targetName string) error {
	_, err := page.Evaluate(`([sourceName, targetName]) => {
		const items = Array.from(document.querySelectorAll('[data-testid="project-item"]'));
		const findItem = (name) => items.find((item) => item.querySelector('[data-testid="project-name"]')?.textContent?.trim() === name);
		const source = findItem(sourceName);
		const target = findItem(targetName);
		if (!source || !target) {
			throw new Error('missing project item');
		}
		const data = new DataTransfer();
		source.dispatchEvent(new DragEvent('dragstart', { bubbles: true, cancelable: true, dataTransfer: data }));
		target.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, dataTransfer: data }));
		target.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: data }));
		source.dispatchEvent(new DragEvent('dragend', { bubbles: true, cancelable: true, dataTransfer: data }));
	}`, []any{sourceName, targetName})
	return err
}

// writeProjectReorderReport 使用 outputDir、success 和 events 参数写入 Project 排序报告。
func writeProjectReorderReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Project 排序 E2E 测试报告", success, events)
}
