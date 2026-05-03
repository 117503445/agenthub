package wsapp

import (
	"os"
	"path/filepath"
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
	if len(options) != 1 {
		t.Fatalf("skill 数量不正确: %#v", options)
	}
	if options[0].ID != "demo-skill" || options[0].Description != "演示 skill。" {
		t.Fatalf("skill 解析不正确: %#v", options[0])
	}
}
