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
	System   any                    `json:"system"`   // System 表示 Claude Code 注入的系统提示。
	Tools    []map[string]any       `json:"tools"`    // Tools 表示请求携带的工具定义。
	Metadata map[string]any         `json:"metadata"` // Metadata 表示可选元数据。
}

// MockAnthropicMessage 表示 Anthropic messages mock 中的一条输入消息。
type MockAnthropicMessage struct {
	Role    string `json:"role"`    // Role 表示消息角色。
	Content any    `json:"content"` // Content 表示消息内容。
}

// MockOpenAIResponsesRequest 表示 OpenAI Responses mock 请求体。
type MockOpenAIResponsesRequest struct {
	Model  string           `json:"model"`  // Model 表示请求模型。
	Stream bool             `json:"stream"` // Stream 表示是否使用 SSE 流式返回。
	Input  any              `json:"input"`  // Input 表示 Responses API 输入。
	Tools  []map[string]any `json:"tools"`  // Tools 表示请求携带的工具定义。
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
	planStage := detectMockPlanStage(mockAnthropicClassificationText(prompt, request))
	log.Ctx(r.Context()).Info().
		Str("planStage", string(planStage)).
		Strs("tools", mockOpenAIToolNames(request.Tools)).
		Int("promptChars", len([]rune(prompt))).
		Msg("收到 Anthropic mock 请求")
	responseText := buildMockResponse("Mock Claude", prompt, planStage)
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

// ServeMockOpenAIResponses 使用 w 和 r 参数提供 OpenAI Responses 兼容接口。
func ServeMockOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request MockOpenAIResponsesRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	prompt := extractMockOpenAIInputPrompt(request.Input)
	if prompt == "" {
		prompt = "空输入"
	}
	planStage := detectMockPlanStage(prompt)
	log.Ctx(r.Context()).Info().
		Str("planStage", string(planStage)).
		Strs("tools", mockOpenAIToolNames(request.Tools)).
		Int("promptChars", len([]rune(prompt))).
		Msg("收到 OpenAI mock 请求")
	if strings.Contains(prompt, "MOCK_AGENT_ERROR") {
		http.Error(w, "mock codex error: forced failure", http.StatusBadRequest)
		return
	}
	responseText := buildMockResponse("Mock Codex", prompt, planStage)
	model := request.Model
	if model == "" {
		model = "gpt-5.5"
	}

	if request.Stream {
		if mockOpenAIShouldRequestUserInput(prompt, request.Input, request.Tools, planStage) {
			serveMockOpenAIRequestUserInputStream(w, r, model)
			return
		}
		if mockOpenAIShouldCallTool(request.Input, request.Tools, planStage) {
			serveMockOpenAIToolCallStream(w, r, model)
			return
		}
		serveMockOpenAIResponsesStream(w, r, model, responseText)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(mockOpenAICompletedResponse(model, responseText))
}

// serveMockOpenAIToolCallStream 使用 w、r 和 model 参数返回一次 exec_command 工具调用。
func serveMockOpenAIToolCallStream(w http.ResponseWriter, r *http.Request, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	responseID := "resp_mock_tool"
	callID := "call_mock_pwd"
	itemID := "fc_mock_pwd"
	arguments := `{"cmd":"pwd","yield_time_ms":1000,"max_output_tokens":2000}`
	sequence := 0
	writeSSE := func(event string, payload any) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("编码 OpenAI mock 工具 SSE 失败")
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	response := mockOpenAIBaseResponse(responseID, model, "in_progress", nil)
	if !writeSSE("response.created", map[string]any{"type": "response.created", "response": response}) {
		return
	}
	if !writeSSE("response.in_progress", map[string]any{"type": "response.in_progress", "response": response}) {
		return
	}
	if !writeSSE("response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item":         mockOpenAIFunctionCallItem(itemID, callID, "exec_command", "", "in_progress"),
	}) {
		return
	}
	sequence++
	if !writeSSE("response.function_call_arguments.delta", map[string]any{
		"type":            "response.function_call_arguments.delta",
		"item_id":         itemID,
		"output_index":    0,
		"delta":           arguments,
		"sequence_number": sequence,
	}) {
		return
	}
	sequence++
	if !writeSSE("response.function_call_arguments.done", map[string]any{
		"type":            "response.function_call_arguments.done",
		"item_id":         itemID,
		"output_index":    0,
		"name":            "exec_command",
		"arguments":       arguments,
		"sequence_number": sequence,
	}) {
		return
	}
	sequence++
	toolItem := mockOpenAIFunctionCallItem(itemID, callID, "exec_command", arguments, "completed")
	if !writeSSE("response.output_item.done", map[string]any{
		"type":            "response.output_item.done",
		"output_index":    0,
		"item":            toolItem,
		"sequence_number": sequence,
	}) {
		return
	}
	sequence++
	_ = writeSSE("response.completed", map[string]any{
		"type":            "response.completed",
		"response":        mockOpenAIBaseResponse(responseID, model, "completed", []any{toolItem}),
		"sequence_number": sequence,
	})
}

// serveMockOpenAIRequestUserInputStream 使用 w、r 和 model 参数返回一次 request_user_input 工具调用。
func serveMockOpenAIRequestUserInputStream(w http.ResponseWriter, r *http.Request, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	responseID := "resp_mock_user_input"
	callID := "call_mock_user_input"
	arguments := `{"questions":[{"id":"confirm_path","header":"Confirm","question":"Proceed with the plan?","options":[{"label":"Yes (Recommended)","description":"Continue the current plan."},{"label":"No","description":"Stop and revisit the approach."}]}]}`
	writeSSE := func(event string, payload any) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("编码 OpenAI mock request_user_input SSE 失败")
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !writeSSE("response.created", map[string]any{
		"type":     "response.created",
		"response": map[string]any{"id": responseID},
	}) {
		return
	}
	toolItem := mockOpenAIFunctionCallItem("fc_mock_user_input", callID, "request_user_input", arguments, "completed")
	if !writeSSE("response.output_item.done", map[string]any{
		"type": "response.output_item.done",
		"item": toolItem,
	}) {
		return
	}
	_ = writeSSE("response.completed", map[string]any{
		"type":     "response.completed",
		"response": mockOpenAIBaseResponse(responseID, model, "completed", []any{toolItem}),
	})
}

// mockOpenAIToolNames 使用 tools 参数提取工具名称列表。
func mockOpenAIToolNames(tools []map[string]any) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		name := firstNonEmpty(stringValue(tool["name"]), stringValue(tool["type"]))
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// mockOpenAIShouldRequestUserInput 使用 prompt、input、tools 和 planStage 参数判断是否需要返回用户输入请求。
func mockOpenAIShouldRequestUserInput(prompt string, input any, tools []map[string]any, planStage mockPlanStage) bool {
	if planStage != mockPlanStageDraft && planStage != mockPlanStageRevise {
		return false
	}
	if !strings.Contains(strings.ToLower(prompt), "request_user_input") {
		return false
	}
	if mockOpenAIHasFunctionOutput(input) {
		return false
	}
	for _, name := range mockOpenAIToolNames(tools) {
		if name == "request_user_input" {
			return true
		}
	}
	return false
}

// mockOpenAIShouldCallTool 使用 input、tools 和 planStage 参数判断是否需要先返回工具调用。
func mockOpenAIShouldCallTool(input any, tools []map[string]any, planStage mockPlanStage) bool {
	if planStage == mockPlanStageDraft || planStage == mockPlanStageRevise {
		return false
	}
	if mockOpenAIHasFunctionOutput(input) {
		return false
	}
	for _, name := range mockOpenAIToolNames(tools) {
		if name == "exec_command" {
			return true
		}
	}
	return false
}

// mockOpenAIHasFunctionOutput 使用 input 参数判断请求是否已经携带工具结果。
func mockOpenAIHasFunctionOutput(input any) bool {
	items, ok := input.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if stringValue(block["type"]) == "function_call_output" {
			return true
		}
	}
	return false
}

// serveMockOpenAIResponsesStream 使用 w、r、model 和 responseText 参数返回 Responses API SSE。
func serveMockOpenAIResponsesStream(w http.ResponseWriter, r *http.Request, model string, responseText string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	responseID := "resp_mock"
	messageID := "msg_mock"
	sequence := 0
	writeSSE := func(event string, payload any) bool {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Msg("编码 OpenAI mock SSE 失败")
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	response := mockOpenAIBaseResponse(responseID, model, "in_progress", nil)
	if !writeSSE("response.created", map[string]any{"type": "response.created", "response": response}) {
		return
	}
	if !writeSSE("response.in_progress", map[string]any{"type": "response.in_progress", "response": response}) {
		return
	}
	if !writeSSE("response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item":         mockOpenAIMessageItem(messageID, "in_progress", ""),
	}) {
		return
	}
	sequence++
	if !writeSSE("response.content_part.added", map[string]any{
		"type":            "response.content_part.added",
		"item_id":         messageID,
		"output_index":    0,
		"content_index":   0,
		"part":            map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
		"sequence_number": sequence,
	}) {
		return
	}

	for _, chunk := range splitMockResponse(responseText) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(120 * time.Millisecond):
		}
		sequence++
		if !writeSSE("response.output_text.delta", map[string]any{
			"type":            "response.output_text.delta",
			"item_id":         messageID,
			"output_index":    0,
			"content_index":   0,
			"delta":           chunk,
			"sequence_number": sequence,
		}) {
			return
		}
	}

	sequence++
	_ = writeSSE("response.output_text.done", map[string]any{
		"type":            "response.output_text.done",
		"item_id":         messageID,
		"output_index":    0,
		"content_index":   0,
		"text":            responseText,
		"sequence_number": sequence,
	})
	sequence++
	_ = writeSSE("response.content_part.done", map[string]any{
		"type":            "response.content_part.done",
		"item_id":         messageID,
		"output_index":    0,
		"content_index":   0,
		"part":            map[string]any{"type": "output_text", "text": responseText, "annotations": []any{}},
		"sequence_number": sequence,
	})
	sequence++
	_ = writeSSE("response.output_item.done", map[string]any{
		"type":            "response.output_item.done",
		"output_index":    0,
		"item":            mockOpenAIMessageItem(messageID, "completed", responseText),
		"sequence_number": sequence,
	})
	sequence++
	_ = writeSSE("response.completed", map[string]any{
		"type":            "response.completed",
		"response":        mockOpenAIBaseResponse(responseID, model, "completed", []any{mockOpenAIMessageItem(messageID, "completed", responseText)}),
		"sequence_number": sequence,
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

// mockPlanStage 表示 mock 模型识别出的 plan 流程阶段。
type mockPlanStage string

const (
	mockPlanStageNone    mockPlanStage = ""
	mockPlanStageDraft   mockPlanStage = "draft"
	mockPlanStageRevise  mockPlanStage = "revise"
	mockPlanStageExecute mockPlanStage = "execute"
)

// detectMockPlanStage 使用 prompt 参数识别 plan 初稿、修订和执行阶段。
func detectMockPlanStage(prompt string) mockPlanStage {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return mockPlanStageNone
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "开始执行已确认的 plan") ||
		strings.Contains(lower, "已确认 plan:") ||
		strings.Contains(lower, "approved plan:") ||
		strings.Contains(lower, "the user approved the plan") {
		return mockPlanStageExecute
	}
	hasPlanModeMarker := strings.Contains(lower, "plan 模式") ||
		strings.Contains(lower, "plan mode") ||
		strings.Contains(lower, "permission-mode plan") ||
		strings.Contains(lower, "exitplanmode") ||
		strings.Contains(lower, "enterplanmode")
	if !hasPlanModeMarker {
		return mockPlanStageNone
	}
	if strings.Contains(trimmed, "先写测试") ||
		strings.Contains(trimmed, "改成") ||
		strings.Contains(trimmed, "修改") ||
		strings.Contains(lower, "revise") ||
		strings.Contains(lower, "update the plan") {
		return mockPlanStageRevise
	}
	return mockPlanStageDraft
}

// mockAnthropicClassificationText 使用 prompt 和 request 参数生成 Claude mock 阶段识别文本。
func mockAnthropicClassificationText(prompt string, request MockAnthropicMessageRequest) string {
	parts := []string{prompt}
	if systemText := strings.TrimSpace(extractMockContentText(request.System)); systemText != "" {
		parts = append(parts, systemText)
	}
	if toolNames := mockOpenAIToolNames(request.Tools); len(toolNames) > 0 {
		parts = append(parts, strings.Join(toolNames, "\n"))
	}
	return strings.Join(parts, "\n")
}

// buildMockResponse 使用 agentLabel、prompt 和 planStage 参数构造 mock 回复文本。
func buildMockResponse(agentLabel string, prompt string, planStage mockPlanStage) string {
	switch planStage {
	case mockPlanStageDraft, mockPlanStageRevise:
		firstStep := "先梳理需求和 plan 约束"
		if planStage == mockPlanStageRevise || strings.Contains(prompt, "先写测试") {
			firstStep = "先写测试覆盖 mock plan 模式"
		}
		return fmt.Sprintf(
			"## %s Plan\n\n待确认意见\n\n- %s。\n- 改写 mock 模型服务，让 %s 在 plan 模式只返回计划。\n- 运行单独 case 和全量 E2E，确认生成、修订、执行都通过。\n\n请确认是否开始执行。",
			agentLabel,
			firstStep,
			agentLabel,
		)
	case mockPlanStageExecute:
		return fmt.Sprintf(
			"## %s 执行结果\n\n开始执行已确认的 plan。\n\n- 已进入执行阶段。\n- 已使用确认后的 plan 继续实现。\n- mock 执行阶段不会再次请求确认。",
			agentLabel,
		)
	default:
		if agentLabel == "Mock Codex" {
			return fmt.Sprintf(
				"## Mock Codex\n\n正在回复：%s\n\n- 来自后端 OpenAI Responses 兼容 mock 服务。\n- 用于验证 Codex CLI 和工具调用展示。",
				prompt,
			)
		}
		return fmt.Sprintf(
			"## Mock Claude\n\n正在回复：%s\n\n- 来自后端 ANTHROPIC 兼容 mock 服务。\n- 用于验证会话恢复和停止发送流程。",
			prompt,
		)
	}
}

// buildMockAnthropicResponse 使用 prompt 参数构造 mock 回复文本。
func buildMockAnthropicResponse(prompt string) string {
	return buildMockResponse("Mock Claude", prompt, detectMockPlanStage(prompt))
}

// buildMockOpenAIResponse 使用 prompt 参数构造 Codex mock 回复文本。
func buildMockOpenAIResponse(prompt string) string {
	return buildMockResponse("Mock Codex", prompt, detectMockPlanStage(prompt))
}

// extractMockOpenAIInputPrompt 使用 input 参数提取 Responses API 最后一条用户文本。
func extractMockOpenAIInputPrompt(input any) string {
	switch typed := input.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		for index := len(typed) - 1; index >= 0; index-- {
			message, ok := typed[index].(map[string]any)
			if !ok {
				continue
			}
			if role := stringValue(message["role"]); role != "" && role != "user" {
				continue
			}
			text := extractMockOpenAIContentText(message["content"])
			if strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
		return ""
	default:
		return ""
	}
}

// extractMockOpenAIContentText 使用 content 参数提取 Responses API 文本。
func extractMockOpenAIContentText(content any) string {
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
			blockType := stringValue(block["type"])
			if blockType != "" && blockType != "input_text" && blockType != "text" {
				continue
			}
			if text := stringValue(block["text"]); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// mockOpenAICompletedResponse 使用 model 和 responseText 参数构造完整 Responses API 响应。
func mockOpenAICompletedResponse(model string, responseText string) map[string]any {
	return mockOpenAIBaseResponse("resp_mock", model, "completed", []any{mockOpenAIMessageItem("msg_mock", "completed", responseText)})
}

// mockOpenAIBaseResponse 使用 id、model、status 和 output 参数构造 Responses API 基础响应。
func mockOpenAIBaseResponse(id string, model string, status string, output []any) map[string]any {
	if output == nil {
		output = []any{}
	}
	return map[string]any{
		"id":                   id,
		"object":               "response",
		"created_at":           time.Now().Unix(),
		"status":               status,
		"error":                nil,
		"incomplete_details":   nil,
		"instructions":         nil,
		"max_output_tokens":    nil,
		"model":                model,
		"output":               output,
		"parallel_tool_calls":  true,
		"previous_response_id": nil,
		"reasoning":            map[string]any{"effort": nil, "summary": nil},
		"store":                false,
		"temperature":          1.0,
		"text":                 map[string]any{"format": map[string]string{"type": "text"}},
		"tool_choice":          "auto",
		"tools":                []any{},
		"top_p":                1.0,
		"truncation":           "disabled",
		"usage":                map[string]any{"input_tokens": 42, "output_tokens": len([]rune(fmt.Sprint(output))) / 2, "total_tokens": 42 + len([]rune(fmt.Sprint(output)))/2},
		"user":                 nil,
		"metadata":             map[string]any{},
	}
}

// mockOpenAIMessageItem 使用 id、status 和 text 参数构造 Responses API 消息项。
func mockOpenAIMessageItem(id string, status string, text string) map[string]any {
	content := []any{}
	if text != "" {
		content = append(content, map[string]any{"type": "output_text", "text": text, "annotations": []any{}})
	}
	return map[string]any{
		"id":      id,
		"type":    "message",
		"status":  status,
		"role":    "assistant",
		"content": content,
	}
}

// mockOpenAIFunctionCallItem 使用 id、callID、name、arguments 和 status 参数构造函数调用项。
func mockOpenAIFunctionCallItem(id string, callID string, name string, arguments string, status string) map[string]any {
	return map[string]any{
		"id":        id,
		"type":      "function_call",
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
		"status":    status,
	}
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
