package wsapp

import (
	"strings"
	"testing"
)

// TestParseClaudeOutputLine 验证 Claude stream-json 输出解析。
func TestParseClaudeOutputLine(t *testing.T) {
	cases := []struct {
		name          string
		line          string
		wantSessionID string
		wantDelta     string
		wantAssistant string
		wantDone      bool
		wantError     string
	}{
		{
			name:      "解析增量文本",
			line:      `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"你好"}}}`,
			wantDelta: "你好",
		},
		{
			name:          "解析 assistant 完整消息",
			line:          `{"type":"assistant","session_id":"session-1","message":{"content":[{"type":"text","text":"完整回复"}]}}`,
			wantSessionID: "session-1",
			wantAssistant: "完整回复",
		},
		{
			name:          "解析成功结果",
			line:          `{"type":"result","session_id":"session-2","result":"最终回复","subtype":"success"}`,
			wantSessionID: "session-2",
			wantAssistant: "最终回复",
			wantDone:      true,
		},
		{
			name:      "解析错误结果",
			line:      `{"type":"result","subtype":"error","error":"失败原因"}`,
			wantError: "失败原因",
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			event, err := parseClaudeOutputLine([]byte(item.line))
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if event.SessionID != item.wantSessionID {
				t.Fatalf("session id 不正确: got=%q want=%q", event.SessionID, item.wantSessionID)
			}
			if event.Delta != item.wantDelta {
				t.Fatalf("delta 不正确: got=%q want=%q", event.Delta, item.wantDelta)
			}
			if event.AssistantText != item.wantAssistant {
				t.Fatalf("assistant 文本不正确: got=%q want=%q", event.AssistantText, item.wantAssistant)
			}
			if event.Done != item.wantDone {
				t.Fatalf("done 不正确: got=%v want=%v", event.Done, item.wantDone)
			}
			if event.Error != item.wantError {
				t.Fatalf("error 不正确: got=%q want=%q", event.Error, item.wantError)
			}
		})
	}
}

// TestBuildClaudeUserMessage 验证发送给 Claude stdin 的用户消息结构。
func TestBuildClaudeUserMessage(t *testing.T) {
	line, err := buildClaudeUserMessage("测试 prompt", "session-1")
	if err != nil {
		t.Fatalf("构造用户消息失败: %v", err)
	}
	if event, err := parseClaudeOutputLine([]byte(line)); err != nil || event.SessionID != "session-1" {
		t.Fatalf("用户消息中的 session id 不正确: event=%#v err=%v", event, err)
	}
	if !containsAll(line, []string{`"type":"user"`, `"text":"测试 prompt"`, `"session_id":"session-1"`}) {
		t.Fatalf("用户消息内容不正确: %s", line)
	}
}

// containsAll 使用 text 和 parts 参数判断所有片段是否存在。
func containsAll(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
