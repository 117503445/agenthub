package wsapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadAgentSkillOptions 验证用户目录中的 SKILL.md 会被解析为输入框候选项。
func TestLoadAgentSkillOptions(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	skillDir := filepath.Join(homeDir, ".codex", "skills", "demo-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建 skill 目录失败: %v", err)
	}
	content := []byte("---\nname: demo-skill\ndescription: 演示 skill。\n---\n\n# Demo\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0644); err != nil {
		t.Fatalf("写入 SKILL.md 失败: %v", err)
	}

	options := LoadAgentSkillOptions(nil)
	byID := agentSkillOptionsByID(options)
	option, ok := byID["demo-skill"]
	if !ok {
		t.Fatalf("未找到 demo-skill: %#v", options)
	}
	if option.Description != "演示 skill。" {
		t.Fatalf("skill 解析不正确: %#v", option)
	}
	if !strings.HasSuffix(option.Path, filepath.Join(".codex", "skills", "demo-skill", "SKILL.md")) {
		t.Fatalf("skill 路径不正确: %#v", option)
	}
}

// TestLoadAgentSkillOptionsOfficialDirs 验证 Claude Code 和 Codex 官方 skill 目录会被扫描。
func TestLoadAgentSkillOptionsOfficialDirs(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	projectDir := t.TempDir()
	mustWriteTestSkill(t, filepath.Join(homeDir, ".claude", "skills"), "home-claude", "用户 Claude skill。")
	mustWriteTestSkill(t, filepath.Join(homeDir, ".agents", "skills"), "home-agents", "用户 Agents skill。")
	mustWriteTestSkill(t, filepath.Join(projectDir, ".claude", "skills"), "project-claude", "项目 Claude skill。")
	mustWriteTestSkill(t, filepath.Join(projectDir, ".agents", "skills"), "project-agents", "项目 Agents skill。")

	options := LoadAgentSkillOptions([]string{projectDir})
	byID := agentSkillOptionsByID(options)
	for _, id := range []string{"home-claude", "home-agents", "project-claude", "project-agents"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("未找到 skill %q: %#v", id, options)
		}
	}
	if !strings.HasSuffix(byID["home-claude"].Path, filepath.Join(".claude", "skills", "home-claude", "SKILL.md")) {
		t.Fatalf("Claude 用户 skill 路径不正确: %#v", byID["home-claude"])
	}
	if !strings.HasSuffix(byID["home-agents"].Path, filepath.Join(".agents", "skills", "home-agents", "SKILL.md")) {
		t.Fatalf("Agents 用户 skill 路径不正确: %#v", byID["home-agents"])
	}
}

// TestFormatAgentSkillsMarkdownIncludesSearchPaths 验证 #skills 回复会包含搜索路径。
func TestFormatAgentSkillsMarkdownIncludesSearchPaths(t *testing.T) {
	markdown := formatAgentSkillsMarkdown([]AgentSkillOption{{
		ID:          "demo-skill",
		Label:       "demo-skill",
		Description: "演示 skill。",
		Path:        filepath.Join("demo", "SKILL.md"),
	}}, []string{filepath.Join("demo", ".agents", "skills")})
	if !strings.Contains(markdown, "## skills 搜索路径") {
		t.Fatalf("#skills 回复缺少搜索路径标题: %s", markdown)
	}
	if !strings.Contains(markdown, filepath.Join("demo", ".agents", "skills")) {
		t.Fatalf("#skills 回复缺少搜索路径: %s", markdown)
	}
}

// mustWriteTestSkill 使用 t、skillsDir、name 和 description 参数写入测试 SKILL.md。
func mustWriteTestSkill(t *testing.T, skillsDir string, name string, description string) {
	t.Helper()
	skillDir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建 skill 目录失败: %v", err)
	}
	content := []byte("---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0644); err != nil {
		t.Fatalf("写入 SKILL.md 失败: %v", err)
	}
}

// agentSkillOptionsByID 使用 options 参数按 ID 索引 skill。
func agentSkillOptionsByID(options []AgentSkillOption) map[string]AgentSkillOption {
	result := make(map[string]AgentSkillOption)
	for _, option := range options {
		result[option.ID] = option
	}
	return result
}
