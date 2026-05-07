package wsapp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// codexAppServerRequestTimeout 表示 app-server 控制请求等待响应的上限。
	codexAppServerRequestTimeout = 30 * time.Second
	// codexRequestUserInputTool 表示 Codex 请求用户输入的工具名。
	codexRequestUserInputTool = "request_user_input"
)

// codexAppRPCMessage 表示 Codex app-server JSON-RPC 消息。
type codexAppRPCMessage struct {
	ID     any               `json:"id,omitempty"`     // ID 表示 JSON-RPC 请求或响应标识。
	Method string            `json:"method,omitempty"` // Method 表示 JSON-RPC 方法名。
	Params json.RawMessage   `json:"params,omitempty"` // Params 表示请求或通知参数。
	Result json.RawMessage   `json:"result,omitempty"` // Result 表示响应结果。
	Error  *codexAppRPCError `json:"error,omitempty"`  // Error 表示响应错误。
}

// codexAppRPCError 表示 Codex app-server JSON-RPC 错误。
type codexAppRPCError struct {
	Code    int    `json:"code"`           // Code 表示 JSON-RPC 错误码。
	Message string `json:"message"`        // Message 表示错误说明。
	Data    any    `json:"data,omitempty"` // Data 表示可选错误详情。
}

// codexAppPendingUserInput 表示等待前端回答的 Codex 用户输入请求。
type codexAppPendingUserInput struct {
	RequestID any                 // RequestID 表示 app-server 原始 JSON-RPC 请求标识。
	Questions []UserInputQuestion // Questions 表示本次请求的问题列表。
	Input     string              // Input 表示展示用问题摘要。
}

// sendCodexApp 使用 ctx 和 input 参数通过 Codex app-server 启动 plan 模式单轮运行。
func (m *AgentManager) sendCodexApp(ctx context.Context, input AgentRunInput) error {
	runtime, err := m.ensureCodexAppRuntime(ctx, input)
	if err != nil {
		return err
	}

	runtime.mu.Lock()
	if runtime.running {
		runtime.mu.Unlock()
		return fmt.Errorf("chat %s 已经有运行中的 agent", input.ChatID)
	}
	runtime.running = true
	runtime.stopping = false
	runtime.currentMessageID = input.AssistantMessageID
	runtime.emittedAssistantText = ""
	runtime.callbacks = input.Callbacks
	sessionID := runtime.sessionID
	runtime.mu.Unlock()

	if input.Callbacks.OnSessionID != nil && strings.TrimSpace(sessionID) != "" && sessionID != input.SessionID {
		input.Callbacks.OnSessionID(sessionID)
	}
	if err := runtime.startCodexAppTurn(ctx, agentPrompt(input.Prompt, input.Images), input.Images); err != nil {
		runtime.mu.Lock()
		runtime.running = false
		runtime.mu.Unlock()
		runtime.cleanupImageFiles()
		return err
	}
	return nil
}

// ensureCodexAppRuntime 使用 ctx 和 input 参数获取或启动 Codex app-server runtime。
func (m *AgentManager) ensureCodexAppRuntime(ctx context.Context, input AgentRunInput) (*AgentRuntime, error) {
	var stale *AgentRuntime
	m.mu.Lock()
	existing := m.runtimes[input.ChatID]
	if existing != nil &&
		existing.appServer &&
		existing.provider == input.Provider &&
		existing.profileSignature() == agentProfileSignature(input.Profile) &&
		existing.model == input.Model &&
		existing.reasoning == input.Reasoning &&
		existing.projectPath == input.ProjectPath &&
		existing.isAlive() {
		m.mu.Unlock()
		return existing, nil
	}
	if existing != nil {
		delete(m.runtimes, input.ChatID)
		stale = existing
	}

	runtimeCtx, cancel := context.WithCancel(m.ctx)
	runtime := &AgentRuntime{
		manager:              m,
		ctx:                  runtimeCtx,
		cancel:               cancel,
		chatID:               input.ChatID,
		provider:             input.Provider,
		profile:              input.Profile,
		model:                input.Model,
		reasoning:            input.Reasoning,
		planMode:             true,
		projectPath:          input.ProjectPath,
		sessionID:            input.SessionID,
		appServer:            true,
		appPendingResponses:  make(map[string]chan codexAppRPCMessage),
		appPendingUserInputs: make(map[string]codexAppPendingUserInput),
	}
	m.runtimes[input.ChatID] = runtime
	m.mu.Unlock()

	if stale != nil {
		stale.stop()
	}
	if err := runtime.startCodexAppServer(ctx); err != nil {
		cancel()
		m.removeRuntime(input.ChatID, runtime)
		return nil, err
	}
	return runtime, nil
}

// stopIdleCodexAppRuntime 使用 chatID 参数停止空闲的 Codex app-server runtime。
func (m *AgentManager) stopIdleCodexAppRuntime(chatID string) {
	m.mu.Lock()
	existing := m.runtimes[chatID]
	if existing == nil {
		m.mu.Unlock()
		return
	}
	existing.mu.Lock()
	shouldStop := existing.appServer && !existing.running
	existing.mu.Unlock()
	if !shouldStop {
		m.mu.Unlock()
		return
	}
	delete(m.runtimes, chatID)
	m.mu.Unlock()
	existing.stop()
}

// RespondUserInput 使用 chatID、toolCallID 和 answers 参数回答 Codex 用户输入请求。
func (m *AgentManager) RespondUserInput(chatID string, toolCallID string, answers map[string][]string) error {
	m.mu.Lock()
	runtime := m.runtimes[chatID]
	m.mu.Unlock()
	if runtime == nil {
		return fmt.Errorf("%w: agent runtime 不存在", ErrNotFound)
	}
	return runtime.respondCodexAppUserInput(toolCallID, answers)
}

// startCodexAppServer 使用 ctx 参数启动并初始化 Codex app-server。
func (r *AgentRuntime) startCodexAppServer(ctx context.Context) error {
	args := append([]string{"app-server"}, r.profile.Args...)
	args = append(args, "--listen", "stdio://")
	command := r.profile.Command
	cmd := exec.CommandContext(r.ctx, command, args...)
	cmd.Dir = r.projectPath
	cmd.Env = r.codexEnv()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("创建 Codex app-server stdin 失败: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("创建 Codex app-server stdout 失败: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("创建 Codex app-server stderr 失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("启动 Codex app-server 失败: %w", err)
	}

	r.mu.Lock()
	r.cmd = cmd
	r.stdin = stdin
	r.stderrDone = make(chan struct{})
	if r.appPendingResponses == nil {
		r.appPendingResponses = make(map[string]chan codexAppRPCMessage)
	}
	if r.appPendingUserInputs == nil {
		r.appPendingUserInputs = make(map[string]codexAppPendingUserInput)
	}
	stderrDone := r.stderrDone
	r.mu.Unlock()

	log.Ctx(ctx).Info().
		Str("chatID", r.chatID).
		Str("command", command).
		Strs("args", args).
		Str("cwd", r.projectPath).
		Str("baseURL", r.codexBaseURL()).
		Str("model", r.model).
		Str("reasoning", r.reasoning).
		Msg("Codex app-server runtime 已启动")

	go r.scanCodexAppStdout(stdout)
	go r.scanStderr(stderr, stderrDone)
	go r.wait()

	requestCtx, cancel := context.WithTimeout(ctx, codexAppServerRequestTimeout)
	defer cancel()
	if _, err := r.codexAppRequest(requestCtx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "agenthub",
			"title":   "AgentHub",
			"version": "agenthub",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}); err != nil {
		return err
	}
	if err := r.codexAppNotify("initialized", nil); err != nil {
		return err
	}
	result, err := r.codexAppRequest(requestCtx, "thread/start", map[string]any{
		"model":          r.model,
		"cwd":            r.projectPath,
		"approvalPolicy": "never",
		"sandbox":        "read-only",
	})
	if err != nil {
		return err
	}
	sessionID := codexAppThreadID(result)
	if sessionID == "" {
		return fmt.Errorf("Codex app-server thread/start 未返回 thread id")
	}
	r.mu.Lock()
	r.sessionID = sessionID
	r.mu.Unlock()
	return nil
}

// startCodexAppTurn 使用 ctx、prompt 和 images 参数启动 Codex app-server 单轮 plan 运行。
func (r *AgentRuntime) startCodexAppTurn(ctx context.Context, prompt string, images []MessageImage) error {
	imagePaths, err := r.prepareImageFiles(images)
	if err != nil {
		return err
	}
	inputItems := make([]map[string]any, 0, len(imagePaths)+1)
	if strings.TrimSpace(prompt) != "" {
		inputItems = append(inputItems, map[string]any{
			"type":          "text",
			"text":          prompt,
			"text_elements": []any{},
		})
	}
	for _, imagePath := range imagePaths {
		inputItems = append(inputItems, map[string]any{
			"type": "localImage",
			"path": imagePath,
		})
	}
	effort := codexAppReasoningEffort(r.reasoning)
	settings := map[string]any{
		"model":                  r.model,
		"reasoning_effort":       nil,
		"developer_instructions": nil,
	}
	if effort != "" {
		settings["reasoning_effort"] = effort
	}
	params := map[string]any{
		"threadId": r.sessionID,
		"input":    inputItems,
		"model":    r.model,
		"collaborationMode": map[string]any{
			"mode":     "plan",
			"settings": settings,
		},
	}
	if effort != "" {
		params["effort"] = effort
	}
	requestCtx, cancel := context.WithTimeout(ctx, codexAppServerRequestTimeout)
	defer cancel()
	result, err := r.codexAppRequest(requestCtx, "turn/start", params)
	if err != nil {
		return err
	}
	if turnID := codexAppTurnID(result); turnID != "" {
		log.Ctx(ctx).Info().Str("chatID", r.chatID).Str("turnID", turnID).Msg("Codex app-server plan turn 已启动")
	}
	return nil
}

// scanCodexAppStdout 使用 stdout 参数读取 Codex app-server JSON-RPC 输出。
func (r *AgentRuntime) scanCodexAppStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		r.consumeCodexAppLine(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		log.Ctx(r.ctx).Warn().Err(err).Str("chatID", r.chatID).Msg("读取 Codex app-server stdout 失败")
	}
}

// consumeCodexAppLine 使用 line 参数解析 Codex app-server JSON-RPC 行。
func (r *AgentRuntime) consumeCodexAppLine(line []byte) {
	var message codexAppRPCMessage
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	if err := decoder.Decode(&message); err != nil {
		text := strings.TrimSpace(string(line))
		if text != "" {
			log.Ctx(r.ctx).Debug().Err(err).Str("chatID", r.chatID).Str("line", text).Msg("忽略非 JSON Codex app-server 输出")
		}
		return
	}
	if message.Method == "" && message.ID != nil {
		r.resolveCodexAppResponse(message)
		return
	}
	if message.Method != "" && message.ID != nil {
		r.handleCodexAppRequest(message)
		return
	}
	if message.Method != "" {
		r.handleCodexAppNotification(message.Method, message.Params)
	}
}

// resolveCodexAppResponse 使用 message 参数唤醒等待中的 JSON-RPC 请求。
func (r *AgentRuntime) resolveCodexAppResponse(message codexAppRPCMessage) {
	key := codexAppRPCIDKey(message.ID)
	r.mu.Lock()
	ch := r.appPendingResponses[key]
	delete(r.appPendingResponses, key)
	r.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- message:
	default:
	}
}

// handleCodexAppRequest 使用 message 参数处理 Codex app-server 主动发来的请求。
func (r *AgentRuntime) handleCodexAppRequest(message codexAppRPCMessage) {
	switch message.Method {
	case "item/tool/requestUserInput":
		if err := r.handleCodexAppUserInputRequest(message); err != nil {
			_ = r.codexAppError(message.ID, -32602, err.Error())
		}
	default:
		_ = r.codexAppError(message.ID, -32601, fmt.Sprintf("AgentHub 暂不支持 Codex app-server 请求: %s", message.Method))
	}
}

// handleCodexAppUserInputRequest 使用 message 参数转发 request_user_input 到前端。
func (r *AgentRuntime) handleCodexAppUserInputRequest(message codexAppRPCMessage) error {
	var params struct {
		ThreadID  string              `json:"threadId"`  // ThreadID 表示 Codex thread 标识。
		TurnID    string              `json:"turnId"`    // TurnID 表示 Codex turn 标识。
		ItemID    string              `json:"itemId"`    // ItemID 表示工具调用标识。
		Questions []UserInputQuestion `json:"questions"` // Questions 表示请求的问题列表。
	}
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return fmt.Errorf("解析 request_user_input 参数失败: %w", err)
	}
	questions := normalizeUserInputQuestions(params.Questions)
	if len(questions) == 0 {
		return fmt.Errorf("request_user_input 缺少有效问题")
	}
	toolID := strings.TrimSpace(params.ItemID)
	if toolID == "" {
		toolID = "request-" + codexAppRPCIDKey(message.ID)
	}
	input := formatUserInputQuestions(questions)
	now := time.Now()
	tool := ToolCall{
		ID:     toolID,
		Name:   codexRequestUserInputTool,
		Status: ToolCallStatusRunning,
		Input:  input,
		UserInputRequest: &UserInputRequest{
			ID:        toolID,
			Questions: questions,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	r.mu.Lock()
	if r.appPendingUserInputs == nil {
		r.appPendingUserInputs = make(map[string]codexAppPendingUserInput)
	}
	r.appPendingUserInputs[toolID] = codexAppPendingUserInput{
		RequestID: message.ID,
		Questions: questions,
		Input:     input,
	}
	callbacks := r.callbacks
	r.mu.Unlock()

	log.Ctx(r.ctx).Info().
		Str("chatID", r.chatID).
		Str("threadID", params.ThreadID).
		Str("turnID", params.TurnID).
		Str("toolCallID", toolID).
		Int("questionCount", len(questions)).
		Msg("Codex app-server 请求用户输入")
	if callbacks.OnToolCall != nil {
		callbacks.OnToolCall(tool)
	}
	return nil
}

// handleCodexAppNotification 使用 method 和 params 参数消费 Codex app-server 通知。
func (r *AgentRuntime) handleCodexAppNotification(method string, params json.RawMessage) {
	event := ClaudeOutputEvent{}
	switch method {
	case "item/agentMessage/delta", "item/plan/delta":
		event.Delta = stringFromJSON(params, "delta")
	case "item/started", "item/completed", "item/updated":
		event = codexAppEventFromItem(method, params)
	case "item/commandExecution/outputDelta":
		if tool := codexAppCommandOutputDelta(params); tool.ID != "" {
			event.ToolCalls = append(event.ToolCalls, tool)
		}
	case "thread/tokenUsage/updated":
		if usage, ok := extractContextWindowUsage(firstNonNil(mapValueFromJSON(params, "tokenUsage"), mapValueFromJSON(params, "token_usage"))); ok {
			event.ContextWindow = usage
		}
	case "turn/completed":
		event = codexAppTurnCompletedEvent(params)
	case "error":
		event.Error = firstNonEmpty(stringFromJSON(mapValueFromJSON(params, "error"), "message"), stringFromJSON(params, "message"), "Codex app-server 运行失败")
	}
	if event.Delta == "" &&
		event.AssistantText == "" &&
		len(event.ToolCalls) == 0 &&
		event.ContextWindow.MaxTokens <= 0 &&
		event.ContextWindow.UsedTokens <= 0 &&
		!event.Done &&
		event.Error == "" {
		return
	}
	r.consumeOutputEvent(event)
	if event.Done || event.Error != "" {
		r.cleanupImageFiles()
	}
}

// respondCodexAppUserInput 使用 toolCallID 和 answers 参数把前端答案写回 Codex app-server。
func (r *AgentRuntime) respondCodexAppUserInput(toolCallID string, answers map[string][]string) error {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return fmt.Errorf("%w: request_user_input 工具调用不能为空", ErrInvalidInput)
	}
	r.mu.Lock()
	pending, ok := r.appPendingUserInputs[toolCallID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: request_user_input 请求不存在", ErrNotFound)
	}
	normalizedAnswers, err := normalizeUserInputAnswers(pending.Questions, answers)
	if err != nil {
		return err
	}
	resultAnswers := make(map[string]any, len(normalizedAnswers))
	for questionID, values := range normalizedAnswers {
		resultAnswers[questionID] = map[string]any{"answers": values}
	}
	if err := r.codexAppResponse(pending.RequestID, map[string]any{"answers": resultAnswers}); err != nil {
		return err
	}

	r.mu.Lock()
	delete(r.appPendingUserInputs, toolCallID)
	callbacks := r.callbacks
	r.mu.Unlock()

	now := time.Now()
	tool := ToolCall{
		ID:     toolCallID,
		Name:   codexRequestUserInputTool,
		Status: ToolCallStatusComplete,
		Input:  pending.Input,
		Output: formatUserInputAnswers(pending.Questions, normalizedAnswers),
		UserInputRequest: &UserInputRequest{
			ID:        toolCallID,
			Questions: pending.Questions,
			Answers:   normalizedAnswers,
			UpdatedAt: now,
		},
	}
	if callbacks.OnToolCall != nil {
		callbacks.OnToolCall(tool)
	}
	return nil
}

// codexAppRequest 使用 ctx、method 和 params 参数发送 JSON-RPC 请求并等待响应。
func (r *AgentRuntime) codexAppRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	r.mu.Lock()
	r.appNextRequestID++
	requestID := r.appNextRequestID
	key := strconv.FormatInt(requestID, 10)
	ch := make(chan codexAppRPCMessage, 1)
	if r.appPendingResponses == nil {
		r.appPendingResponses = make(map[string]chan codexAppRPCMessage)
	}
	r.appPendingResponses[key] = ch
	r.mu.Unlock()

	if err := r.codexAppWrite(map[string]any{"id": requestID, "method": method, "params": params}); err != nil {
		r.mu.Lock()
		delete(r.appPendingResponses, key)
		r.mu.Unlock()
		return nil, err
	}
	select {
	case <-ctx.Done():
		r.mu.Lock()
		delete(r.appPendingResponses, key)
		r.mu.Unlock()
		return nil, ctx.Err()
	case <-r.ctx.Done():
		r.mu.Lock()
		delete(r.appPendingResponses, key)
		r.mu.Unlock()
		return nil, r.ctx.Err()
	case response := <-ch:
		if response.Error != nil {
			return nil, fmt.Errorf("Codex app-server %s 失败: %s", method, response.Error.Message)
		}
		return response.Result, nil
	}
}

// codexAppNotify 使用 method 和 params 参数发送 JSON-RPC 通知。
func (r *AgentRuntime) codexAppNotify(method string, params any) error {
	message := map[string]any{"method": method}
	if params != nil {
		message["params"] = params
	}
	return r.codexAppWrite(message)
}

// codexAppResponse 使用 id 和 result 参数发送 JSON-RPC 响应。
func (r *AgentRuntime) codexAppResponse(id any, result any) error {
	return r.codexAppWrite(map[string]any{"id": id, "result": result})
}

// codexAppError 使用 id、code 和 message 参数发送 JSON-RPC 错误响应。
func (r *AgentRuntime) codexAppError(id any, code int, message string) error {
	return r.codexAppWrite(map[string]any{
		"id": id,
		"error": codexAppRPCError{
			Code:    code,
			Message: message,
		},
	})
}

// codexAppWrite 使用 message 参数向 Codex app-server stdin 写入一行 JSON。
func (r *AgentRuntime) codexAppWrite(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	r.appWriteMu.Lock()
	defer r.appWriteMu.Unlock()
	r.mu.Lock()
	stdin := r.stdin
	r.mu.Unlock()
	if stdin == nil {
		return fmt.Errorf("Codex app-server stdin 不可用")
	}
	if _, err := stdin.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// codexAppThreadID 使用 result 参数提取 thread/start 响应中的 thread id。
func codexAppThreadID(result json.RawMessage) string {
	thread := mapValueFromJSON(result, "thread")
	return stringValue(thread["id"])
}

// codexAppTurnID 使用 result 参数提取 turn/start 响应中的 turn id。
func codexAppTurnID(result json.RawMessage) string {
	turn := mapValueFromJSON(result, "turn")
	return stringValue(turn["id"])
}

// codexAppEventFromItem 使用 method 和 params 参数从 item 通知中提取输出事件。
func codexAppEventFromItem(method string, params json.RawMessage) ClaudeOutputEvent {
	item := mapValueFromJSON(params, "item")
	if item == nil {
		return ClaudeOutputEvent{}
	}
	itemType := stringValue(item["type"])
	event := ClaudeOutputEvent{}
	switch itemType {
	case "agentMessage":
		event.AssistantText = stringValue(item["text"])
	case "plan":
		event.AssistantText = stringValue(item["text"])
	case "commandExecution":
		status := ToolCallStatusRunning
		switch stringValue(item["status"]) {
		case "completed":
			status = ToolCallStatusComplete
		case "failed", "declined":
			status = ToolCallStatusError
		}
		event.ToolCalls = append(event.ToolCalls, ToolCall{
			ID:     firstNonEmpty(stringValue(item["id"]), "codex-command"),
			Name:   "exec_command",
			Status: status,
			Input:  stringValue(item["command"]),
			Output: stringValue(item["aggregatedOutput"]),
		})
	case "mcpToolCall":
		status := ToolCallStatusRunning
		switch stringValue(item["status"]) {
		case "completed":
			status = ToolCallStatusComplete
		case "failed":
			status = ToolCallStatusError
		}
		event.ToolCalls = append(event.ToolCalls, ToolCall{
			ID:     firstNonEmpty(stringValue(item["id"]), "codex-mcp-tool"),
			Name:   firstNonEmpty(stringValue(item["tool"]), "mcp_tool"),
			Status: status,
			Input:  jsonString(item["arguments"]),
			Output: firstNonEmpty(jsonString(item["result"]), stringFromJSON(mapValue(item["error"]), "message")),
		})
	case "reasoning":
		if text := strings.Join(stringSliceValue(item["summary"]), "\n"); text != "" {
			event.ToolCalls = append(event.ToolCalls, ToolCall{
				ID:     firstNonEmpty(stringValue(item["id"]), "codex-reasoning"),
				Name:   "thinking",
				Status: ToolCallStatusComplete,
				Input:  text,
			})
		}
	}
	if method == "item/started" {
		for index := range event.ToolCalls {
			event.ToolCalls[index].Status = ToolCallStatusRunning
		}
	}
	return event
}

// codexAppCommandOutputDelta 使用 params 参数提取命令输出增量。
func codexAppCommandOutputDelta(params json.RawMessage) ToolCall {
	itemID := stringFromJSON(params, "itemId")
	delta := stringFromJSON(params, "delta")
	if itemID == "" || delta == "" {
		return ToolCall{}
	}
	return ToolCall{
		ID:     itemID,
		Name:   "exec_command",
		Status: ToolCallStatusRunning,
		Output: delta,
	}
}

// codexAppTurnCompletedEvent 使用 params 参数提取 turn 完成或失败事件。
func codexAppTurnCompletedEvent(params json.RawMessage) ClaudeOutputEvent {
	turn := mapValueFromJSON(params, "turn")
	status := stringValue(turn["status"])
	if status == "failed" {
		return ClaudeOutputEvent{Error: firstNonEmpty(stringFromJSON(mapValue(turn["error"]), "message"), "Codex app-server 运行失败")}
	}
	return ClaudeOutputEvent{Done: true}
}

// normalizeUserInputQuestions 使用 questions 参数清理 request_user_input 问题。
func normalizeUserInputQuestions(questions []UserInputQuestion) []UserInputQuestion {
	result := make([]UserInputQuestion, 0, len(questions))
	for _, question := range questions {
		question.ID = strings.TrimSpace(question.ID)
		question.Header = strings.TrimSpace(question.Header)
		question.Question = strings.TrimSpace(question.Question)
		if question.ID == "" || question.Header == "" || question.Question == "" {
			continue
		}
		options := make([]UserInputOption, 0, len(question.Options))
		for _, option := range question.Options {
			option.Label = strings.TrimSpace(option.Label)
			option.Description = strings.TrimSpace(option.Description)
			if option.Label == "" {
				continue
			}
			options = append(options, option)
		}
		question.Options = options
		result = append(result, question)
	}
	return result
}

// normalizeUserInputAnswers 使用 questions 和 answers 参数校验并按问题 ID 整理答案。
func normalizeUserInputAnswers(questions []UserInputQuestion, answers map[string][]string) (map[string][]string, error) {
	result := make(map[string][]string, len(questions))
	for _, question := range questions {
		values := firstStringList(answers[question.ID], answers[question.Header])
		cleaned := make([]string, 0, len(values))
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		if len(cleaned) == 0 {
			return nil, fmt.Errorf("%w: 问题未回答: %s", ErrInvalidInput, question.Header)
		}
		result[question.ID] = cleaned
	}
	return result, nil
}

// formatUserInputQuestions 使用 questions 参数生成问题摘要。
func formatUserInputQuestions(questions []UserInputQuestion) string {
	blocks := make([]string, 0, len(questions))
	for _, question := range questions {
		lines := []string{fmt.Sprintf("%s: %s", question.Header, question.Question)}
		if len(question.Options) > 0 {
			labels := make([]string, 0, len(question.Options))
			for _, option := range question.Options {
				labels = append(labels, option.Label)
			}
			lines = append(lines, "Options: "+strings.Join(labels, ", "))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n"))
}

// formatUserInputAnswers 使用 questions 和 answers 参数生成答案摘要。
func formatUserInputAnswers(questions []UserInputQuestion, answers map[string][]string) string {
	lines := make([]string, 0, len(questions))
	for _, question := range questions {
		values := answers[question.ID]
		if question.IsSecret {
			lines = append(lines, fmt.Sprintf("%s: 已提交", question.Header))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", question.Header, strings.Join(values, ", ")))
	}
	return strings.Join(lines, "\n")
}

// firstStringList 使用 values 参数返回第一个非空字符串列表。
func firstStringList(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

// codexAppReasoningEffort 使用 reasoning 参数返回 app-server 可接受的推理级别。
func codexAppReasoningEffort(reasoning string) string {
	switch strings.ToLower(strings.TrimSpace(reasoning)) {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(reasoning))
	default:
		return ""
	}
}

// codexAppRPCIDKey 使用 id 参数生成稳定的请求标识 key。
func codexAppRPCIDKey(id any) string {
	switch typed := id.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

// mapValueFromJSON 使用 value 和 key 参数从 JSON 对象中读取 map 字段。
func mapValueFromJSON(value any, key string) map[string]any {
	raw, ok := rawJSONValue(value)
	if !ok {
		return nil
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil
	}
	if key == "" {
		return object
	}
	return mapValue(object[key])
}

// stringFromJSON 使用 value 和 key 参数从 JSON 对象中读取字符串字段。
func stringFromJSON(value any, key string) string {
	if mapped := mapValue(value); mapped != nil {
		return stringValue(mapped[key])
	}
	raw, ok := rawJSONValue(value)
	if !ok {
		return ""
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return ""
	}
	return stringValue(object[key])
}

// rawJSONValue 使用 value 参数返回可解码 JSON 字节。
func rawJSONValue(value any) ([]byte, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case json.RawMessage:
		return typed, true
	case []byte:
		return typed, true
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil, false
		}
		return data, true
	}
}

// stringSliceValue 使用 value 参数读取字符串切片。
func stringSliceValue(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text := stringValue(item); text != "" {
			result = append(result, text)
		}
	}
	return result
}
