package e2e

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/playwright-community/playwright-go"
)

// runSkillCommandCase 使用 ctx 参数运行 skill 目录扫描和 #skills 命令 E2E 用例。
func runSkillCommandCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeAgentChatReport(ctx.OutputDir, success, events)
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
		ctx.Logger.Errorf("Skill 命令 E2E 失败: %v", err)
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

	homeDir := filepath.Join(ctx.RootDir, "data", "e2e", "home")
	claudeSkillName := "e2e-claude-skill"
	agentsSkillName := "e2e-agents-skill"
	if err := writeE2ESkill(filepath.Join(homeDir, ".claude", "skills"), claudeSkillName, "E2E Claude 目录 Skill。"); err != nil {
		return fail(fmt.Errorf("写入 Claude skill 失败: %w", err))
	}
	if err := writeE2ESkill(filepath.Join(homeDir, ".agents", "skills"), agentsSkillName, "E2E Agents 目录 Skill。"); err != nil {
		return fail(fmt.Errorf("写入 Agents skill 失败: %w", err))
	}

	if err := fillTestID(page, "message-input", "/"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "skill-menu", claudeSkillName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "skill-menu", agentsSkillName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := fillTestID(page, "message-input", ""); err != nil {
		return fail(err)
	}

	if err := fillTestID(page, "message-input", "#"); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "skill-menu", "skills", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}
	if err := expectTestIDValue(page, "message-input", "#skills ", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		return fail(err)
	}

	if err := expectTestIDText(page, "message-log", "可用 skills", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", claudeSkillName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", "E2E Claude 目录 Skill。", 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", filepath.Join(".claude", "skills", claudeSkillName, "SKILL.md"), 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", agentsSkillName, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "message-log", filepath.Join(".agents", "skills", agentsSkillName, "SKILL.md"), 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectSkillTable(page, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDNotText(page, "message-log", "Mock Codex", 2*time.Second); err != nil {
		return fail(err)
	}

	events = append(events, reportStep("可从 Claude Code 和 Codex 用户 skill 目录发现 SKILL.md；#skills 命令通过本地表格回复，不启动 agent。"))
	return true
}

// expectSkillTable 使用 page 和 timeout 参数等待 assistant markdown 中出现 skills 表格。
func expectSkillTable(page playwright.Page, timeout time.Duration) error {
	return waitForCondition("等待 skills 表格", timeout, func() (bool, string, error) {
		count, err := page.Locator(`[data-testid="assistant-markdown"] table`).Count()
		if err != nil {
			return false, "", err
		}
		return count > 0, fmt.Sprintf("实际数量: %d", count), nil
	})
}
