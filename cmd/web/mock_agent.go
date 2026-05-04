package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// mockAgentMessage 表示 mock Anthropic 请求消息。
type mockAgentMessage struct {
	Role    string `json:"role"`    // Role 表示消息角色。
	Content any    `json:"content"` // Content 表示消息内容。
}

// mockAgentRequest 表示 mock Anthropic 请求体。
type mockAgentRequest struct {
	Model    string             `json:"model"`    // Model 表示模型名称。
	Stream   bool               `json:"stream"`   // Stream 表示是否流式返回。
	Messages []mockAgentMessage `json:"messages"` // Messages 表示输入消息列表。
}

// mockAgentResponse 表示 mock Anthropic 响应体。
type mockAgentResponse struct {
	Content []struct {
		Type string `json:"type"` // Type 表示 content block 类型。
		Text string `json:"text"` // Text 表示 content block 文本。
	} `json:"content"` // Content 表示响应内容。
}

// mockOpenAIResponse 表示 mock OpenAI Responses 响应体。
type mockOpenAIResponse struct {
	Output []mockOpenAIOutput `json:"output"` // Output 表示模型输出项列表。
}

// mockOpenAIOutput 表示 mock OpenAI Responses 的单个输出项。
type mockOpenAIOutput struct {
	Type    string              `json:"type"`    // Type 表示输出项类型。
	Content []mockOpenAIContent `json:"content"` // Content 表示输出文本内容。
}

// mockOpenAIContent 表示 mock OpenAI Responses 的输出内容块。
type mockOpenAIContent struct {
	Type string `json:"type"` // Type 表示内容块类型。
	Text string `json:"text"` // Text 表示文本内容。
}

// runMockAgentCLIIfRequested 使用 args 参数判断并运行 E2E mock agent CLI。
func runMockAgentCLIIfRequested(args []string) bool {
	mode := mockAgentMode(args)
	switch mode {
	case "mock-codex":
		os.Exit(runMockCodexCLI(os.Args[1:], os.Stdout, os.Stderr))
	case "mock-claude":
		os.Exit(runMockClaudeCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	default:
		return false
	}
	return true
}

// mockAgentMode 使用 args 参数返回当前 mock agent 模式。
func mockAgentMode(args []string) string {
	name := filepath.Base(args[0])
	if name == "mock-codex" || name == "mock-claude" {
		return name
	}
	if len(args) > 1 && (args[1] == "mock-codex" || args[1] == "mock-claude") {
		return args[1]
	}
	return ""
}

// runMockCodexCLI 使用 args、stdout 和 stderr 参数运行 Codex JSONL mock。
func runMockCodexCLI(args []string, stdout io.Writer, stderr io.Writer) int {
	model := mockCLIModelFromArgs(args)
	prompt := mockCLILastArg(args)
	writer := bufio.NewWriter(stdout)
	threadID := fmt.Sprintf("mock-codex-%d", time.Now().UnixNano())
	writeMockJSONLine(writer, map[string]any{"type": "thread.started", "thread_id": threadID})
	writeMockJSONLine(writer, map[string]any{"type": "turn.started"})
	writeMockJSONLine(writer, map[string]any{
		"type": "item.started",
		"item": map[string]any{
			"id":                "mock-codex-request",
			"type":              "command_execution",
			"command":           "pwd",
			"aggregated_output": "",
			"status":            "in_progress",
		},
	})
	_ = writer.Flush()

	time.Sleep(600 * time.Millisecond)
	if strings.Contains(prompt, "MOCK_AGENT_ERROR") {
		_, _ = fmt.Fprintln(stderr, "mock codex error: forced failure")
		return 1
	}
	text, err := requestMockCodexText(model, prompt)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mock codex request failed: %v\n", err)
		return 1
	}

	writeMockJSONLine(writer, map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":                "mock-codex-request",
			"type":              "command_execution",
			"command":           "pwd",
			"aggregated_output": ".",
			"exit_code":         0,
			"status":            "completed",
		},
	})
	writeMockJSONLine(writer, map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "mock-codex-message",
			"type": "agent_message",
			"text": text,
		},
	})
	writeMockJSONLine(writer, map[string]any{"type": "turn.completed"})
	_ = writer.Flush()
	// 让父进程先消费 stdout，避免极快退出时先收到进程完成回调。
	time.Sleep(300 * time.Millisecond)
	return 0
}

// requestMockCodexText 使用 model 和 prompt 参数请求 OpenAI 兼容 mock 模型服务。
func requestMockCodexText(model string, prompt string) (string, error) {
	if model == "" {
		model = "mock-model"
	}
	payload := map[string]any{
		"model":  model,
		"stream": false,
		"input":  prompt,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	response, err := http.Post(mockOpenAIResponsesURL(), "application/json", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("mock 服务状态码 %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result mockOpenAIResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	for _, output := range result.Output {
		if output.Type != "message" {
			continue
		}
		for _, content := range output.Content {
			if content.Type == "output_text" && content.Text != "" {
				return content.Text, nil
			}
		}
	}
	return "", fmt.Errorf("mock 服务没有返回文本")
}

// runMockClaudeCLI 使用 args、stdin、stdout 和 stderr 参数运行 Claude stream-json mock。
func runMockClaudeCLI(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	model := mockCLIModelFromArgs(args)
	planMode := mockCLIPlanMode(args)
	scanner := bufio.NewScanner(stdin)
	writer := bufio.NewWriter(stdout)
	for scanner.Scan() {
		prompt, sessionID := extractMockClaudePrompt(scanner.Bytes())
		if strings.TrimSpace(prompt) == "" {
			continue
		}
		if sessionID == "" {
			sessionID = fmt.Sprintf("mock-claude-%d", time.Now().UnixNano())
		}
		if strings.Contains(prompt, "MOCK_AGENT_ERROR") {
			writeMockJSONLine(writer, map[string]any{"type": "result", "session_id": sessionID, "subtype": "error", "error": "mock claude error: forced failure"})
			_ = writer.Flush()
			continue
		}
		if planMode {
			prompt = mockPlanModePrompt(prompt)
		}
		text, err := requestMockAgentText(model, prompt)
		if err != nil {
			writeMockJSONLine(writer, map[string]any{"type": "result", "session_id": sessionID, "subtype": "error", "error": err.Error()})
			_ = writer.Flush()
			continue
		}
		writeMockClaudeToolStart(writer, sessionID)
		for _, chunk := range mockCLISplitText(text) {
			writeMockJSONLine(writer, map[string]any{
				"type":       "stream_event",
				"session_id": sessionID,
				"event": map[string]any{
					"type":  "content_block_delta",
					"delta": map[string]string{"type": "text_delta", "text": chunk},
				},
			})
			_ = writer.Flush()
			time.Sleep(80 * time.Millisecond)
		}
		writeMockClaudeToolResult(writer, sessionID)
		writeMockJSONLine(writer, map[string]any{"type": "result", "session_id": sessionID, "subtype": "success"})
		_ = writer.Flush()
	}
	if err := scanner.Err(); err != nil {
		_, _ = fmt.Fprintf(stderr, "mock claude stdin failed: %v\n", err)
		return 1
	}
	return 0
}

// mockPlanModePrompt 使用 prompt 参数生成 mock CLI 的 plan 模式提示。
func mockPlanModePrompt(prompt string) string {
	return strings.Join([]string{
		"你现在处于 plan 模式。只阅读、分析并生成可执行计划，不要修改文件或执行实现。",
		"如果计划需要用户确认，请把待确认意见写清楚。",
		"用户批准后才可以开始执行。",
		"",
		prompt,
	}, "\n")
}

// mockCLIPlanMode 使用 args 参数判断 mock Claude CLI 是否运行在 plan 模式。
func mockCLIPlanMode(args []string) bool {
	for index := 0; index < len(args)-1; index++ {
		if args[index] == "--permission-mode" && args[index+1] == "plan" {
			return true
		}
	}
	return false
}

// requestMockAgentText 使用 model 和 prompt 参数请求服务端 mock 模型服务。
func requestMockAgentText(model string, prompt string) (string, error) {
	if model == "" {
		model = "mock-model"
	}
	payload := mockAgentRequest{
		Model:  model,
		Stream: false,
		Messages: []mockAgentMessage{{
			Role:    "user",
			Content: []map[string]string{{"type": "text", "text": prompt}},
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	response, err := http.Post(mockAgentMessagesURL(), "application/json", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("mock 服务状态码 %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result mockAgentResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	for _, item := range result.Content {
		if item.Type == "text" && item.Text != "" {
			return item.Text, nil
		}
	}
	return "", fmt.Errorf("mock 服务没有返回文本")
}

// mockAgentMessagesURL 返回 mock Anthropic messages 端点地址。
func mockAgentMessagesURL() string {
	baseURL := defaultAgentHubBaseURL(os.Getenv("AGENTHUB_PORT")) + "/mock/anthropic"
	return baseURL + "/v1/messages"
}

// mockOpenAIResponsesURL 返回 mock OpenAI Responses 端点地址。
func mockOpenAIResponsesURL() string {
	return defaultAgentHubBaseURL(os.Getenv("AGENTHUB_PORT")) + "/mock/openai/v1/responses"
}

// mockCLIModelFromArgs 使用 args 参数提取 --model 的值。
func mockCLIModelFromArgs(args []string) string {
	for index := 0; index < len(args)-1; index++ {
		if args[index] == "--model" || args[index] == "-m" {
			return args[index+1]
		}
	}
	return "sonnet"
}

// mockCLILastArg 使用 args 参数提取最后一个非空参数。
func mockCLILastArg(args []string) string {
	for index := len(args) - 1; index >= 0; index-- {
		value := strings.TrimSpace(args[index])
		if value != "" && !strings.HasPrefix(value, "-") {
			return value
		}
	}
	return ""
}

// extractMockClaudePrompt 使用 line 参数提取 Claude stream-json 用户输入。
func extractMockClaudePrompt(line []byte) (string, string) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return "", ""
	}
	sessionID, _ := raw["session_id"].(string)
	message, _ := raw["message"].(map[string]any)
	if message == nil {
		return "", sessionID
	}
	return mockCLIContentText(message["content"]), sessionID
}

// mockCLIContentText 使用 value 参数提取 Claude content 文本。
func mockCLIContentText(value any) string {
	switch typed := value.(type) {
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

// writeMockClaudeToolStart 使用 writer 和 sessionID 参数输出 Claude 工具调用开始事件。
func writeMockClaudeToolStart(writer *bufio.Writer, sessionID string) {
	writeMockJSONLine(writer, map[string]any{
		"type":       "assistant",
		"session_id": sessionID,
		"message": map[string]any{
			"content": []map[string]any{{
				"type":  "tool_use",
				"id":    "mock-claude-request",
				"name":  "HTTP",
				"input": map[string]string{"url": mockAgentMessagesURL()},
			}},
		},
	})
}

// writeMockClaudeToolResult 使用 writer 和 sessionID 参数输出 Claude 工具调用结果事件。
func writeMockClaudeToolResult(writer *bufio.Writer, sessionID string) {
	writeMockJSONLine(writer, map[string]any{
		"type":       "user",
		"session_id": sessionID,
		"message": map[string]any{
			"content": []map[string]string{{
				"type":        "tool_result",
				"tool_use_id": "mock-claude-request",
				"content":     "mock model request complete",
			}},
		},
	})
}

// writeMockJSONLine 使用 writer 和 payload 参数输出一行 JSON。
func writeMockJSONLine(writer *bufio.Writer, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = writer.Write(data)
	_ = writer.WriteByte('\n')
}

// mockCLISplitText 使用 text 参数拆分 mock 输出片段。
func mockCLISplitText(text string) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return []string{""}
	}
	chunks := make([]string, 0, len(runes)/8+1)
	for start := 0; start < len(runes); start += 8 {
		end := start + 8
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}
