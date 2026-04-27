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
	if len(response.Content) != 1 || !strings.Contains(response.Content[0].Text, "Mock Claude 正在回复：你好") {
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
	if !strings.Contains(text, "event: content_block_delta") || !strings.Contains(text, "Mock Cla") {
		t.Fatalf("缺少流式文本事件: %s", text)
	}
	if !strings.Contains(text, "event: message_stop") {
		t.Fatalf("缺少 message_stop 事件: %s", text)
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
