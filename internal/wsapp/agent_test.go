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
		wantToolName  string
		wantToolInput string
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
			name:          "解析 Claude 工具调用",
			line:          `{"type":"assistant","session_id":"session-1","message":{"content":[{"type":"tool_use","id":"tool-1","name":"Read","input":{"file_path":"README.md"}}]}}`,
			wantSessionID: "session-1",
			wantToolName:  "Read",
			wantToolInput: `{"file_path":"README.md"}`,
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
			if item.wantToolName != "" {
				if len(event.ToolCalls) != 1 {
					t.Fatalf("工具调用数量不正确: %#v", event.ToolCalls)
				}
				if event.ToolCalls[0].Name != item.wantToolName || event.ToolCalls[0].Input != item.wantToolInput {
					t.Fatalf("工具调用不正确: %#v", event.ToolCalls[0])
				}
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

// TestParseCodexOutputLine 验证 Codex JSONL 输出解析。
func TestParseCodexOutputLine(t *testing.T) {
	event, err := parseCodexOutputLine([]byte(`{"type":"thread.started","thread_id":"thread-1"}`))
	if err != nil {
		t.Fatalf("解析 Codex session 失败: %v", err)
	}
	if event.SessionID != "thread-1" {
		t.Fatalf("Codex session 不正确: %#v", event)
	}

	event, err = parseCodexOutputLine([]byte(`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"你好"}}`))
	if err != nil {
		t.Fatalf("解析 Codex 文本失败: %v", err)
	}
	if event.AssistantText != "你好" {
		t.Fatalf("Codex assistant 文本不正确: %#v", event)
	}

	event, err = parseCodexOutputLine([]byte(`{"type":"item.completed","item":{"id":"cmd-1","type":"command_execution","command":"go test ./...","aggregated_output":"ok","status":"completed"}}`))
	if err != nil {
		t.Fatalf("解析 Codex 工具失败: %v", err)
	}
	if len(event.ToolCalls) != 1 || event.ToolCalls[0].Name != "exec_command" || event.ToolCalls[0].Output != "ok" || event.ToolCalls[0].Status != ToolCallStatusComplete {
		t.Fatalf("Codex 工具调用不正确: %#v", event.ToolCalls)
	}

	event, err = parseCodexOutputLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":1}}`))
	if err != nil {
		t.Fatalf("解析 Codex 完成事件失败: %v", err)
	}
	if !event.Done {
		t.Fatalf("Codex 完成事件不正确: %#v", event)
	}

	event, err = parseCodexOutputLine([]byte(`{"type":"error","message":"mock codex error: forced failure\n"}`))
	if err != nil {
		t.Fatalf("解析 Codex 错误事件失败: %v", err)
	}
	if event.Error != "mock codex error: forced failure\n" {
		t.Fatalf("Codex 错误事件不正确: %#v", event)
	}

	event, err = parseCodexOutputLine([]byte(`{"type":"turn.failed","error":{"message":"mock codex error: forced failure\n"}}`))
	if err != nil {
		t.Fatalf("解析 Codex 失败事件失败: %v", err)
	}
	if event.Error != "mock codex error: forced failure\n" {
		t.Fatalf("Codex 失败事件不正确: %#v", event)
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
