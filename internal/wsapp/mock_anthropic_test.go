package wsapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServeMockAnthropicMessagesNonStream 验证非流式 mock messages 响应。
func TestServeMockAnthropicMessagesNonStream(t *testing.T) {
	body := strings.NewReader(`{"model":"sonnet","messages":[{"role":"user","content":[{"type":"text","text":"你好"}]}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	recorder := httptest.NewRecorder()

	ServeMockAnthropicMessages(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码不正确: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Model   string `json:"model"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if response.Model != "sonnet" {
		t.Fatalf("模型不正确: %s", response.Model)
	}
	if len(response.Content) != 1 || !strings.Contains(response.Content[0].Text, "## Mock Claude") || !strings.Contains(response.Content[0].Text, "正在回复：你好") {
		t.Fatalf("响应文本不正确: %#v", response.Content)
	}
}

// TestServeMockAnthropicMessagesStream 验证流式 mock messages 响应。
func TestServeMockAnthropicMessagesStream(t *testing.T) {
	body := strings.NewReader(`{"model":"sonnet","stream":true,"messages":[{"role":"user","content":"流式"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", body)
	recorder := httptest.NewRecorder()

	ServeMockAnthropicMessages(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码不正确: %d body=%s", recorder.Code, recorder.Body.String())
	}
	text := recorder.Body.String()
	if !strings.Contains(text, "event: message_start") {
		t.Fatalf("缺少 message_start 事件: %s", text)
	}
	if !strings.Contains(text, "event: content_block_delta") || !strings.Contains(text, "## Mock ") {
		t.Fatalf("缺少流式文本事件: %s", text)
	}
	if !strings.Contains(text, "event: message_stop") {
		t.Fatalf("缺少 message_stop 事件: %s", text)
	}
}

// TestBuildMockAnthropicPlanResponses 验证 Claude mock 会按 plan 阶段返回稳定内容。
func TestBuildMockAnthropicPlanResponses(t *testing.T) {
	planText := buildMockAnthropicResponse(planModePrompt("生成 mock claude plan 模式测试"))
	if !strings.Contains(planText, "Mock Claude Plan") || !strings.Contains(planText, "待确认意见") {
		t.Fatalf("Claude plan 响应不正确: %s", planText)
	}

	revisedText := buildMockAnthropicResponse(planModePrompt("请把第一条改成先写测试"))
	if !strings.Contains(revisedText, "Mock Claude Plan") || !strings.Contains(revisedText, "先写测试") {
		t.Fatalf("Claude plan 修订响应不正确: %s", revisedText)
	}

	executedText := buildMockAnthropicResponse("开始执行已确认的 plan。\n\n已确认 plan:\n\n- 先写测试")
	if !strings.Contains(executedText, "Mock Claude 执行结果") || strings.Contains(executedText, "待确认意见") {
		t.Fatalf("Claude plan 执行响应不正确: %s", executedText)
	}
}

// TestServeMockAnthropicMessagesPlanToolMarker 验证 Claude mock 可从官方 plan 工具标记识别 plan 阶段。
func TestServeMockAnthropicMessagesPlanToolMarker(t *testing.T) {
	body := strings.NewReader(`{"model":"sonnet","messages":[{"role":"user","content":"生成计划"}],"tools":[{"name":"ExitPlanMode"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	recorder := httptest.NewRecorder()

	ServeMockAnthropicMessages(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码不正确: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Mock Claude Plan") {
		t.Fatalf("未按 ExitPlanMode 识别 plan 阶段: %s", recorder.Body.String())
	}
}

// TestExtractLastUserPromptStripsSystemReminder 验证 mock 会移除 Claude Code 注入的系统提醒。
func TestExtractLastUserPromptStripsSystemReminder(t *testing.T) {
	prompt := extractLastUserPrompt([]MockAnthropicMessage{
		{Role: "user", Content: "<system-reminder>日期</system-reminder>\n\n真实 prompt"},
	})
	if prompt != "真实 prompt" {
		t.Fatalf("prompt 提取不正确: %q", prompt)
	}
}

// TestServeMockOpenAIResponsesNonStream 验证非流式 OpenAI Responses mock 响应。
func TestServeMockOpenAIResponsesNonStream(t *testing.T) {
	body := strings.NewReader(`{"model":"gpt-5.5","input":[{"role":"user","content":[{"type":"input_text","text":"你好 Codex"}]}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	recorder := httptest.NewRecorder()

	ServeMockOpenAIResponses(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码不正确: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Model  string `json:"model"`
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if response.Model != "gpt-5.5" {
		t.Fatalf("模型不正确: %s", response.Model)
	}
	if len(response.Output) != 1 || len(response.Output[0].Content) != 1 || !strings.Contains(response.Output[0].Content[0].Text, "## Mock Codex") || !strings.Contains(response.Output[0].Content[0].Text, "正在回复：你好 Codex") {
		t.Fatalf("响应文本不正确: %#v", response.Output)
	}
}

// TestServeMockOpenAIResponsesStream 验证流式 OpenAI Responses mock 响应。
func TestServeMockOpenAIResponsesStream(t *testing.T) {
	body := strings.NewReader(`{"model":"gpt-5.5","stream":true,"input":"流式 Codex"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	recorder := httptest.NewRecorder()

	ServeMockOpenAIResponses(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码不正确: %d body=%s", recorder.Code, recorder.Body.String())
	}
	text := recorder.Body.String()
	if !strings.Contains(text, "event: response.created") {
		t.Fatalf("缺少 response.created 事件: %s", text)
	}
	if !strings.Contains(text, "event: response.output_text.delta") || !strings.Contains(text, "## Mock ") {
		t.Fatalf("缺少流式文本事件: %s", text)
	}
	if !strings.Contains(text, "event: response.completed") {
		t.Fatalf("缺少 response.completed 事件: %s", text)
	}
}

// TestBuildMockOpenAIPlanResponses 验证 Codex mock 会按 plan 阶段返回稳定内容。
func TestBuildMockOpenAIPlanResponses(t *testing.T) {
	planText := buildMockOpenAIResponse(planModePrompt("生成 mock codex plan 模式测试"))
	if !strings.Contains(planText, "Mock Codex Plan") || !strings.Contains(planText, "待确认意见") {
		t.Fatalf("Codex plan 响应不正确: %s", planText)
	}

	revisedText := buildMockOpenAIResponse(planModePrompt("请把第一条改成先写测试"))
	if !strings.Contains(revisedText, "Mock Codex Plan") || !strings.Contains(revisedText, "先写测试") {
		t.Fatalf("Codex plan 修订响应不正确: %s", revisedText)
	}

	executedText := buildMockOpenAIResponse("开始执行已确认的 plan。\n\n已确认 plan:\n\n- 先写测试")
	if !strings.Contains(executedText, "Mock Codex 执行结果") || strings.Contains(executedText, "待确认意见") {
		t.Fatalf("Codex plan 执行响应不正确: %s", executedText)
	}
}

// TestServeMockOpenAIResponsesPlanPromptSkipsToolCall 验证 Codex mock plan 阶段不会先返回工具调用。
func TestServeMockOpenAIResponsesPlanPromptSkipsToolCall(t *testing.T) {
	requestBody, err := json.Marshal(MockOpenAIResponsesRequest{
		Model:  "gpt-5.5",
		Stream: true,
		Input:  planModePrompt("生成 mock codex plan 模式测试"),
		Tools:  []map[string]any{{"type": "function", "name": "exec_command"}},
	})
	if err != nil {
		t.Fatalf("编码请求失败: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(requestBody)))
	recorder := httptest.NewRecorder()

	ServeMockOpenAIResponses(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码不正确: %d body=%s", recorder.Code, recorder.Body.String())
	}
	text := recorder.Body.String()
	if strings.Contains(text, "response.function_call_arguments.delta") {
		t.Fatalf("plan 阶段不应返回工具调用: %s", text)
	}
	if !strings.Contains(text, "Mock Codex Plan") {
		t.Fatalf("缺少 Codex plan 文本: %s", text)
	}
}
