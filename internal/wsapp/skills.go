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
}

// LoadAgentSkillOptions 使用 projectPaths 参数扫描可用 skill 列表。
func LoadAgentSkillOptions(projectPaths []string) []AgentSkillOption {
	dirs := skillCandidateDirs(projectPaths)
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

// skillCandidateDirs 使用 projectPaths 参数返回需要扫描的 skill 目录。
func skillCandidateDirs(projectPaths []string) []string {
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
		addDir(filepath.Join(trimmed, ".codex", "skills"))
		if repoRoot, err := gitOutput(trimmed, "rev-parse", "--show-toplevel"); err == nil {
			addDir(filepath.Join(repoRoot, ".codex", "skills"))
		}
	}

	codexHome := ""
	if homeDir, err := os.UserHomeDir(); err == nil {
		codexHome = filepath.Join(homeDir, ".codex")
	}
	addDir(filepath.Join(codexHome, "skills"))
	return dirs
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
	return AgentSkillOption{ID: name, Label: name, Description: description}, true
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
