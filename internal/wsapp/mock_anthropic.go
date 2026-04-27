package wsapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// MockAnthropicMessageRequest 表示 Anthropic messages mock 请求体。
type MockAnthropicMessageRequest struct {
	Model    string                 `json:"model"`    // Model 表示请求模型。
	Stream   bool                   `json:"stream"`   // Stream 表示是否使用 SSE 流式返回。
	Messages []MockAnthropicMessage `json:"messages"` // Messages 表示请求消息列表。
	Metadata map[string]any         `json:"metadata"` // Metadata 表示可选元数据。
}

// MockAnthropicMessage 表示 Anthropic messages mock 中的一条输入消息。
type MockAnthropicMessage struct {
	Role    string `json:"role"`    // Role 表示消息角色。
	Content any    `json:"content"` // Content 表示消息内容。
}

// ServeMockAnthropicCountTokens 使用 w 和 r 参数返回固定 token 统计。
func ServeMockAnthropicCountTokens(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"input_tokens": 42,
	})
}

// ServeMockAnthropicMessages 使用 w 和 r 参数提供 Anthropic messages 兼容接口。
func ServeMockAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request MockAnthropicMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	prompt := extractLastUserPrompt(request.Messages)
	if prompt == "" {
		prompt = "空输入"
	}
	responseText := buildMockAnthropicResponse(prompt)
	model := request.Model
	if model == "" {
		model = "sonnet"
	}

	if request.Stream {
		serveMockAnthropicStream(w, r, model, responseText)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":            "msg_mock",
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       []map[string]string{{"type": "text", "text": responseText}},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage":         map[string]int{"input_tokens": 42, "output_tokens": len([]rune(responseText)) / 2},
	})
}

// serveMockAnthropicStream 使用 w、r、model 和 responseText 参数返回 SSE 流。
func serveMockAnthropicStream(w http.ResponseWriter, r *http.Request, model string, responseText string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeSSE := func(event string, payload any) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("编码 Anthropic mock SSE 失败")
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !writeSSE("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            "msg_mock_stream",
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]int{"input_tokens": 42, "output_tokens": 0},
		},
	}) {
		return
	}
	if !writeSSE("content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]string{"type": "text", "text": ""},
	}) {
		return
	}

	for _, chunk := range splitMockResponse(responseText) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(160 * time.Millisecond):
		}
		if !writeSSE("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]string{"type": "text_delta", "text": chunk},
		}) {
			return
		}
	}

	_ = writeSSE("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": 0,
	})
	_ = writeSSE("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": len([]rune(responseText)) / 2},
	})
	_ = writeSSE("message_stop", map[string]any{
		"type": "message_stop",
	})
}

// extractLastUserPrompt 使用 messages 参数提取最后一条用户消息。
func extractLastUserPrompt(messages []MockAnthropicMessage) string {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role != "user" {
			continue
		}
		text := extractMockContentText(message.Content)
		if strings.TrimSpace(text) != "" {
			return stripMockSystemReminder(strings.TrimSpace(text))
		}
	}
	return ""
}

// extractMockContentText 使用 content 参数提取文本内容。
func extractMockContentText(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := block["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// stripMockSystemReminder 使用 text 参数移除 Claude Code 注入的系统提醒前缀。
func stripMockSystemReminder(text string) string {
	trimmed := strings.TrimSpace(text)
	for strings.HasPrefix(trimmed, "<system-reminder>") {
		end := strings.Index(trimmed, "</system-reminder>")
		if end == -1 {
			return trimmed
		}
		trimmed = strings.TrimSpace(trimmed[end+len("</system-reminder>"):])
	}
	return trimmed
}

// buildMockAnthropicResponse 使用 prompt 参数构造 mock 回复文本。
func buildMockAnthropicResponse(prompt string) string {
	return fmt.Sprintf(
		"Mock Claude 正在回复：%s\n\n这是来自后端 ANTHROPIC 兼容 mock 服务的流式内容，用于验证会话恢复和停止发送流程。",
		prompt,
	)
}

// splitMockResponse 使用 text 参数拆分 mock 流式输出片段。
func splitMockResponse(text string) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return []string{""}
	}
	chunkSize := 8
	chunks := make([]string, 0, len(runes)/chunkSize+1)
	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}
