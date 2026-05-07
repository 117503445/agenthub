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
		os.Exit(runMockCodexCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
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

// mockCodexAppRPCMessage 表示 mock Codex app-server JSON-RPC 消息。
type mockCodexAppRPCMessage struct {
	ID     any             `json:"id,omitempty"`     // ID 表示请求或响应标识。
	Method string          `json:"method,omitempty"` // Method 表示 JSON-RPC 方法名。
	Params json.RawMessage `json:"params,omitempty"` // Params 表示请求参数。
	Result json.RawMessage `json:"result,omitempty"` // Result 表示响应结果。
	Error  any             `json:"error,omitempty"`  // Error 表示响应错误。
}

// mockCodexAppPendingTurn 表示等待用户输入响应的 mock Codex turn。
type mockCodexAppPendingTurn struct {
	ThreadID string // ThreadID 表示 Codex thread 标识。
	TurnID   string // TurnID 表示 Codex turn 标识。
	Model    string // Model 表示本轮模型。
	Prompt   string // Prompt 表示本轮用户输入。
	PlanMode bool   // PlanMode 表示本轮是否运行在 plan 模式。
}

// runMockCodexCLI 使用 args、stdin、stdout 和 stderr 参数运行 Codex mock。
func runMockCodexCLI(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if mockCodexAppServerArgs(args) {
		return runMockCodexAppServerCLI(args, stdin, stdout, stderr)
	}
	model := mockCLIModelFromArgs(args)
	if mockCodexDashPromptWithoutSeparator(args) {
		_, _ = fmt.Fprintf(stderr, "error: unexpected argument %q found\n", mockCLIRawLastArg(args))
		return 2
	}
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

// mockCodexAppServerArgs 使用 args 参数判断是否启动 app-server mock。
func mockCodexAppServerArgs(args []string) bool {
	for _, arg := range args {
		if arg == "app-server" {
			return true
		}
	}
	return false
}

// runMockCodexAppServerCLI 使用 args、stdin、stdout 和 stderr 参数运行 Codex app-server mock。
func runMockCodexAppServerCLI(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	writer := bufio.NewWriter(stdout)
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	threadID := fmt.Sprintf("mock-codex-app-thread-%d", time.Now().UnixNano())
	pendingTurns := map[string]mockCodexAppPendingTurn{}
	for scanner.Scan() {
		var message mockCodexAppRPCMessage
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.UseNumber()
		if err := decoder.Decode(&message); err != nil {
			continue
		}
		if message.Method == "" && message.ID != nil {
			mockCodexAppHandleClientResponse(writer, pendingTurns, message)
			_ = writer.Flush()
			continue
		}
		switch message.Method {
		case "initialize":
			writeMockCodexAppResponse(writer, message.ID, map[string]any{
				"userAgent":      "mock-codex-app-server",
				"codexHome":      ".",
				"platformFamily": "unix",
				"platformOs":     "linux",
			})
		case "initialized":
			// initialized 是客户端通知，不需要响应。
		case "thread/start":
			writeMockCodexAppResponse(writer, message.ID, map[string]any{
				"thread": map[string]any{"id": threadID},
			})
		case "turn/start":
			mockCodexAppStartTurn(writer, pendingTurns, threadID, message)
		default:
			if message.ID != nil {
				writeMockCodexAppError(writer, message.ID, -32601, "mock Codex app-server 不支持该方法")
			}
		}
		_ = writer.Flush()
	}
	if err := scanner.Err(); err != nil {
		_, _ = fmt.Fprintf(stderr, "mock codex app-server stdin failed: %v\n", err)
		return 1
	}
	_ = args
	return 0
}

// mockCodexAppStartTurn 使用 writer、pendingTurns、threadID 和 message 参数启动一个 mock turn。
func mockCodexAppStartTurn(writer *bufio.Writer, pendingTurns map[string]mockCodexAppPendingTurn, threadID string, message mockCodexAppRPCMessage) {
	model, prompt, planMode := mockCodexAppTurnInput(message.Params)
	turnID := fmt.Sprintf("mock-codex-app-turn-%d", time.Now().UnixNano())
	writeMockCodexAppResponse(writer, message.ID, map[string]any{
		"turn": map[string]any{
			"id":     turnID,
			"status": "inProgress",
		},
	})
	_ = writer.Flush()
	if !planMode {
		writeMockCodexAppCommand(writer, threadID, turnID, "inProgress", "")
		_ = writer.Flush()
	}
	time.Sleep(600 * time.Millisecond)
	if strings.Contains(prompt, "MOCK_AGENT_ERROR") {
		writeMockCodexAppNotify(writer, "turn/completed", map[string]any{
			"threadId": threadID,
			"turn": map[string]any{
				"id":     turnID,
				"status": "failed",
				"error":  map[string]string{"message": "mock codex error: forced failure"},
			},
		})
		return
	}
	if planMode && strings.Contains(prompt, "request_user_input") {
		requestID := fmt.Sprintf("mock-request-user-input-%d", time.Now().UnixNano())
		pendingTurns[requestID] = mockCodexAppPendingTurn{
			ThreadID: threadID,
			TurnID:   turnID,
			Model:    model,
			Prompt:   prompt,
			PlanMode: planMode,
		}
		writeMockCodexAppRequest(writer, requestID, "item/tool/requestUserInput", map[string]any{
			"threadId": threadID,
			"turnId":   turnID,
			"itemId":   "call_mock_user_input",
			"questions": []map[string]any{{
				"id":       "confirm_path",
				"header":   "Confirm",
				"question": "Proceed with the plan?",
				"options": []map[string]string{{
					"label":       "Yes (Recommended)",
					"description": "Continue generating the mock plan.",
				}, {
					"label":       "No",
					"description": "Stop this mock plan turn.",
				}},
			}},
		})
		return
	}
	mockCodexAppFinishTurn(writer, mockCodexAppPendingTurn{
		ThreadID: threadID,
		TurnID:   turnID,
		Model:    model,
		Prompt:   prompt,
		PlanMode: planMode,
	})
}

// mockCodexAppHandleClientResponse 使用 writer、pendingTurns 和 message 参数处理客户端响应。
func mockCodexAppHandleClientResponse(writer *bufio.Writer, pendingTurns map[string]mockCodexAppPendingTurn, message mockCodexAppRPCMessage) {
	key := mockCodexAppIDKey(message.ID)
	pending, ok := pendingTurns[key]
	if !ok {
		return
	}
	delete(pendingTurns, key)
	writeMockCodexAppNotify(writer, "serverRequest/resolved", map[string]any{
		"threadId":  pending.ThreadID,
		"requestId": message.ID,
	})
	mockCodexAppFinishTurn(writer, pending)
}

// mockCodexAppFinishTurn 使用 writer 和 pending 参数输出 mock plan 并完成 turn。
func mockCodexAppFinishTurn(writer *bufio.Writer, pending mockCodexAppPendingTurn) {
	prompt := pending.Prompt
	if pending.PlanMode {
		prompt = mockPlanModePrompt(prompt)
	}
	if !pending.PlanMode && strings.Contains(prompt, "MOCK_CODEX_DELTA_BURST") {
		writeMockCodexAppCommand(writer, pending.ThreadID, pending.TurnID, "completed", ".")
		writeMockCodexAppBurstDeltas(writer, pending.ThreadID, pending.TurnID)
		writeMockCodexAppNotify(writer, "turn/completed", map[string]any{
			"threadId": pending.ThreadID,
			"turn": map[string]any{
				"id":     pending.TurnID,
				"status": "completed",
			},
		})
		return
	}
	text, err := requestMockCodexText(pending.Model, prompt)
	if err != nil {
		writeMockCodexAppNotify(writer, "turn/completed", map[string]any{
			"threadId": pending.ThreadID,
			"turn": map[string]any{
				"id":     pending.TurnID,
				"status": "failed",
				"error":  map[string]string{"message": err.Error()},
			},
		})
		return
	}
	if !pending.PlanMode {
		writeMockCodexAppCommand(writer, pending.ThreadID, pending.TurnID, "completed", ".")
	}
	itemType := "agentMessage"
	itemID := "mock-codex-app-message"
	if pending.PlanMode {
		itemType = "plan"
		itemID = "mock-codex-app-plan"
	}
	writeMockCodexAppNotify(writer, "item/completed", map[string]any{
		"threadId": pending.ThreadID,
		"turnId":   pending.TurnID,
		"item": map[string]any{
			"id":   itemID,
			"type": itemType,
			"text": text,
		},
	})
	writeMockCodexAppNotify(writer, "turn/completed", map[string]any{
		"threadId": pending.ThreadID,
		"turn": map[string]any{
			"id":     pending.TurnID,
			"status": "completed",
		},
	})
}

// writeMockCodexAppBurstDeltas 使用 writer、threadID 和 turnID 参数输出高频文本 delta。
func writeMockCodexAppBurstDeltas(writer *bufio.Writer, threadID string, turnID string) {
	chunks := []string{
		"Codex ", "流式", "合并", "输出", "完成", "：",
		"这", "些", "碎片", "会", "被", "后端", "合并", "后", "再", "推送", "到", "前端", "。",
	}
	for _, chunk := range chunks {
		writeMockCodexAppNotify(writer, "item/agentMessage/delta", map[string]any{
			"threadId": threadID,
			"turnId":   turnID,
			"itemId":   "mock-codex-app-burst-message",
			"delta":    chunk,
		})
	}
}

// mockCodexAppTurnInput 使用 params 参数提取模型、用户输入和 plan 模式标记。
func mockCodexAppTurnInput(params json.RawMessage) (string, string, bool) {
	var payload struct {
		Model string `json:"model"` // Model 表示本轮模型。
		Input []struct {
			Type string `json:"type"` // Type 表示输入项类型。
			Text string `json:"text"` // Text 表示文本输入。
		} `json:"input"` // Input 表示输入项列表。
		CollaborationMode struct {
			Mode string `json:"mode"` // Mode 表示协作模式名称。
		} `json:"collaborationMode"` // CollaborationMode 表示 app-server 协作模式。
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return "", "", false
	}
	parts := make([]string, 0, len(payload.Input))
	for _, item := range payload.Input {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, item.Text)
		}
	}
	return payload.Model, strings.Join(parts, "\n"), payload.CollaborationMode.Mode == "plan"
}

// writeMockCodexAppRequest 使用 writer、id、method 和 params 参数输出 app-server 请求。
func writeMockCodexAppRequest(writer *bufio.Writer, id any, method string, params any) {
	writeMockJSONLine(writer, map[string]any{"id": id, "method": method, "params": params})
}

// writeMockCodexAppNotify 使用 writer、method 和 params 参数输出 app-server 通知。
func writeMockCodexAppNotify(writer *bufio.Writer, method string, params any) {
	writeMockJSONLine(writer, map[string]any{"method": method, "params": params})
}

// writeMockCodexAppResponse 使用 writer、id 和 result 参数输出 app-server 响应。
func writeMockCodexAppResponse(writer *bufio.Writer, id any, result any) {
	writeMockJSONLine(writer, map[string]any{"id": id, "result": result})
}

// writeMockCodexAppError 使用 writer、id、code 和 message 参数输出 app-server 错误。
func writeMockCodexAppError(writer *bufio.Writer, id any, code int, message string) {
	writeMockJSONLine(writer, map[string]any{
		"id": id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

// writeMockCodexAppCommand 使用 writer、threadID、turnID、status 和 output 参数输出命令事件。
func writeMockCodexAppCommand(writer *bufio.Writer, threadID string, turnID string, status string, output string) {
	method := "item/started"
	if status == "completed" {
		method = "item/completed"
	}
	writeMockCodexAppNotify(writer, method, map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
		"item": map[string]any{
			"id":               "mock-codex-app-command",
			"type":             "commandExecution",
			"command":          "pwd",
			"aggregatedOutput": output,
			"status":           status,
		},
	})
}

// mockCodexAppIDKey 使用 id 参数生成 pending map key。
func mockCodexAppIDKey(id any) string {
	switch typed := id.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return fmt.Sprint(typed)
	}
}

// mockCodexDashPromptWithoutSeparator 使用 args 参数模拟 Codex resume 对 dash prompt 的参数校验。
func mockCodexDashPromptWithoutSeparator(args []string) bool {
	prompt := mockCLIRawLastArg(args)
	if !strings.HasPrefix(prompt, "-") {
		return false
	}
	for index := 0; index < len(args)-1; index += 1 {
		if args[index] != "resume" {
			continue
		}
		for promptIndex := len(args) - 1; promptIndex > index; promptIndex -= 1 {
			if strings.TrimSpace(args[promptIndex]) == "" {
				continue
			}
			return promptIndex > 0 && args[promptIndex-1] != "--"
		}
	}
	return false
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
	if prompt, ok := mockCLIAfterDoubleDash(args); ok {
		return prompt
	}
	for index := len(args) - 1; index >= 0; index-- {
		value := strings.TrimSpace(args[index])
		if value != "" && !strings.HasPrefix(value, "-") {
			return value
		}
	}
	return ""
}

// mockCLIAfterDoubleDash 使用 args 参数提取 -- 分隔符后的 prompt。
func mockCLIAfterDoubleDash(args []string) (string, bool) {
	for index := len(args) - 2; index >= 0; index-- {
		if args[index] != "--" {
			continue
		}
		prompt := strings.TrimSpace(args[index+1])
		return prompt, prompt != ""
	}
	return "", false
}

// mockCLIRawLastArg 使用 args 参数提取最后一个非空参数。
func mockCLIRawLastArg(args []string) string {
	for index := len(args) - 1; index >= 0; index-- {
		value := strings.TrimSpace(args[index])
		if value != "" {
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
