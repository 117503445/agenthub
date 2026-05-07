package wsapp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AgentSkillOption 表示聊天输入框中可选择的 skill。
type AgentSkillOption struct {
	ID          string `json:"id"`          // ID 表示 skill 命令标识。
	Label       string `json:"label"`       // Label 表示界面展示名称。
	Description string `json:"description"` // Description 表示 skill 说明。
	Path        string `json:"path"`        // Path 表示 SKILL.md 文件路径。
}

// LoadAgentSkillOptions 使用 projectPaths 参数扫描可用 skill 列表。
func LoadAgentSkillOptions(projectPaths []string) []AgentSkillOption {
	dirs := AgentSkillSearchPaths(projectPaths)
	byID := make(map[string]AgentSkillOption)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			option, ok := readSkillOption(filepath.Join(dir, entry.Name(), "SKILL.md"))
			if !ok {
				continue
			}
			if _, exists := byID[option.ID]; !exists {
				byID[option.ID] = option
			}
		}
	}

	options := make([]AgentSkillOption, 0, len(byID))
	for _, option := range byID {
		options = append(options, option)
	}
	sort.Slice(options, func(i int, j int) bool {
		return options[i].ID < options[j].ID
	})
	return options
}

// AgentSkillSearchPaths 使用 projectPaths 参数返回需要扫描的 skill 目录。
func AgentSkillSearchPaths(projectPaths []string) []string {
	dirs := make([]string, 0)
	seen := make(map[string]struct{})
	addDir := func(dir string) {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			return
		}
		cleaned := filepath.Clean(trimmed)
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		dirs = append(dirs, cleaned)
	}

	for _, projectPath := range projectPaths {
		trimmed := strings.TrimSpace(projectPath)
		if trimmed == "" {
			continue
		}
		addProjectSkillDirs(addDir, trimmed)
	}

	if homeDir, err := os.UserHomeDir(); err == nil {
		addDir(filepath.Join(homeDir, ".claude", "skills"))
		addDir(filepath.Join(homeDir, ".agents", "skills"))
		addDir(filepath.Join(homeDir, ".codex", "skills"))
	}
	addDir(filepath.Join(string(os.PathSeparator), "etc", "codex", "skills"))
	return dirs
}

// addProjectSkillDirs 使用 addDir 和 projectPath 参数添加项目相关 skill 目录。
func addProjectSkillDirs(addDir func(string), projectPath string) {
	cleaned := filepath.Clean(projectPath)
	repoRoot, repoErr := gitOutput(cleaned, "rev-parse", "--show-toplevel")
	if repoErr != nil {
		addDir(filepath.Join(cleaned, ".claude", "skills"))
		addDir(filepath.Join(cleaned, ".agents", "skills"))
		addDir(filepath.Join(cleaned, ".codex", "skills"))
		return
	}

	addDir(filepath.Join(cleaned, ".claude", "skills"))
	addAgentSkillDirsToRepoRoot(addDir, cleaned, filepath.Clean(repoRoot))
	addDir(filepath.Join(cleaned, ".codex", "skills"))
	if filepath.Clean(repoRoot) != cleaned {
		addDir(filepath.Join(repoRoot, ".claude", "skills"))
		addDir(filepath.Join(repoRoot, ".codex", "skills"))
	}
}

// addAgentSkillDirsToRepoRoot 使用 addDir、projectPath 和 repoRoot 参数添加 Codex 官方 .agents/skills 搜索链。
func addAgentSkillDirsToRepoRoot(addDir func(string), projectPath string, repoRoot string) {
	current := filepath.Clean(projectPath)
	root := filepath.Clean(repoRoot)
	for {
		addDir(filepath.Join(current, ".agents", "skills"))
		if current == root {
			return
		}
		parent := filepath.Dir(current)
		if parent == current {
			return
		}
		current = parent
	}
}

// readSkillOption 使用 path 参数读取单个 SKILL.md 的元数据。
func readSkillOption(path string) (AgentSkillOption, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentSkillOption{}, false
	}
	meta := parseSkillFrontMatter(string(data))
	name := strings.TrimSpace(meta["name"])
	description := strings.TrimSpace(meta["description"])
	if name == "" || description == "" {
		return AgentSkillOption{}, false
	}
	return AgentSkillOption{ID: name, Label: name, Description: description, Path: filepath.Clean(path)}, true
}

// parseSkillFrontMatter 使用 content 参数解析 SKILL.md 顶部 front matter。
func parseSkillFrontMatter(content string) map[string]string {
	result := make(map[string]string)
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return result
	}
	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
			break
		}
	}
	if end == -1 {
		return result
	}
	for _, line := range lines[1:end] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		index := strings.Index(trimmed, ":")
		if index <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:index])
		value := strings.TrimSpace(trimmed[index+1:])
		value = strings.Trim(value, `"'`)
		if key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

// isAgentSkillsCommand 使用 prompt 参数判断是否是本地 skills 列表命令。
func isAgentSkillsCommand(prompt string) bool {
	return strings.TrimSpace(prompt) == "#skills"
}

// formatAgentSkillsMarkdown 使用 skills 和 searchPaths 参数生成 #skills 命令回复的 markdown。
func formatAgentSkillsMarkdown(skills []AgentSkillOption, searchPaths []string) string {
	lines := []string{
		"## 可用 skills",
		"",
		"| name | 概要 | 路径 |",
		"| --- | --- | --- |",
	}
	if len(skills) == 0 {
		lines = append(lines, "| - | 暂无可用 skills | - |")
	}
	for _, skill := range skills {
		lines = append(lines, strings.Join([]string{
			"| " + markdownTableCell(skill.ID),
			markdownTableCell(skill.Description),
			markdownTableCell(skill.Path) + " |",
		}, " | "))
	}
	lines = append(lines, "", "## skills 搜索路径", "")
	if len(searchPaths) == 0 {
		lines = append(lines, "- 暂无搜索路径")
		return strings.Join(lines, "\n")
	}
	for _, searchPath := range searchPaths {
		lines = append(lines, "- `"+strings.ReplaceAll(searchPath, "`", "\\`")+"`")
	}
	return strings.Join(lines, "\n")
}

// markdownTableCell 使用 value 参数生成安全的 markdown 表格单元格。
func markdownTableCell(value string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	trimmed = strings.ReplaceAll(trimmed, "\n", " ")
	trimmed = strings.ReplaceAll(trimmed, "|", "\\|")
	if trimmed == "" {
		return "-"
	}
	return trimmed
}
