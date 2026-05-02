package wsapp

import "strings"

const (
	// maxChatTitleRunes 表示自动派生聊天页标题的最大字符数。
	maxChatTitleRunes = 40
)

// deriveChatTitleFromPrompt 使用 prompt 参数从首个非空行派生聊天页标题。
func deriveChatTitleFromPrompt(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		normalized := strings.Join(strings.Fields(line), " ")
		if normalized == "" {
			continue
		}
		return clampChatTitle(normalized)
	}
	return ""
}

// clampChatTitle 使用 title 参数截断聊天页标题。
func clampChatTitle(title string) string {
	runes := []rune(title)
	if len(runes) <= maxChatTitleRunes {
		return title
	}
	return strings.TrimSpace(string(runes[:maxChatTitleRunes]))
}
