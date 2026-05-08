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

// codexAppCollaborationMode 表示 Codex app-server 暴露的协作模式。
type codexAppCollaborationMode struct {
	Name                  string // Name 表示协作模式展示名称。
	Mode                  string // Mode 表示 turn/start 使用的模式标识。
	Model                 string // Model 表示该模式推荐模型。
	ReasoningEffort       string // ReasoningEffort 表示该模式推荐推理级别。
	DeveloperInstructions string // DeveloperInstructions 表示该模式附带的开发者指令。
}

// sendCodexApp 使用 ctx 和 input 参数通过 Codex app-server 启动单轮运行。
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
	runtime.planMode = input.PlanMode
	runtime.currentMessageID = input.AssistantMessageID
	runtime.emittedAssistantText = ""
	runtime.appReasoningByItem = make(map[string]string)
	runtime.callbacks = input.Callbacks
	sessionID := runtime.sessionID
	runtime.mu.Unlock()

	if input.Callbacks.OnSessionID != nil && strings.TrimSpace(sessionID) != "" && sessionID != input.SessionID {
		input.Callbacks.OnSessionID(sessionID)
	}
	runtime.emitCodexAppDiscoveredState(input.Callbacks)
	if err := runtime.startCodexAppTurn(ctx, agentPrompt(input.Prompt, input.Images), input.Images, input.OutputSchema); err != nil {
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
		existing.planMode == input.PlanMode &&
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
		planMode:             input.PlanMode,
		projectPath:          input.ProjectPath,
		sessionID:            input.SessionID,
		appServer:            true,
		appPendingResponses:  make(map[string]chan codexAppRPCMessage),
		appPendingUserInputs: make(map[string]codexAppPendingUserInput),
		appReasoningByItem:   make(map[string]string),
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
	if r.appReasoningByItem == nil {
		r.appReasoningByItem = make(map[string]string)
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
	r.loadCodexAppCapabilities(requestCtx)
	resumeSessionID := strings.TrimSpace(r.sessionID)
	if resumeSessionID != "" {
		if err := r.resumeCodexAppThread(requestCtx, resumeSessionID); err == nil {
			r.loadCodexAppThreadHistory(requestCtx, resumeSessionID)
			return nil
		} else {
			log.Ctx(ctx).Warn().Err(err).Str("chatID", r.chatID).Str("threadID", resumeSessionID).Msg("恢复 Codex app-server thread 失败，将创建新 thread")
		}
	}
	result, err := r.codexAppRequest(requestCtx, "thread/start", map[string]any{
		"model":          r.model,
		"cwd":            r.projectPath,
		"approvalPolicy": "never",
		"sandbox":        "danger-full-access",
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

// resumeCodexAppThread 使用 ctx 和 sessionID 参数恢复 Codex app-server thread。
func (r *AgentRuntime) resumeCodexAppThread(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("Codex app-server resume 缺少 thread id")
	}
	params := map[string]any{
		"threadId": sessionID,
		"model":    r.model,
		"cwd":      r.projectPath,
	}
	if r.codexAppThreadAlreadyLoaded(ctx, sessionID) {
		r.mu.Lock()
		r.sessionID = sessionID
		r.mu.Unlock()
		return nil
	}
	if _, err := r.codexAppRequest(ctx, "thread/resume", params); err != nil {
		return err
	}
	r.mu.Lock()
	r.sessionID = sessionID
	r.mu.Unlock()
	return nil
}

// loadCodexAppCapabilities 使用 ctx 参数读取 Codex app-server 暴露的能力。
func (r *AgentRuntime) loadCodexAppCapabilities(ctx context.Context) {
	r.loadCodexAppCollaborationModes(ctx)
	r.refreshCodexAppSkills(ctx)
	r.loadCodexAppModels(ctx)
}

// loadCodexAppCollaborationModes 使用 ctx 参数读取协作模式列表。
func (r *AgentRuntime) loadCodexAppCollaborationModes(ctx context.Context) {
	result, err := r.codexAppRequest(ctx, "collaborationMode/list", map[string]any{})
	if err != nil {
		log.Ctx(ctx).Debug().Err(err).Str("chatID", r.chatID).Msg("读取 Codex collaboration modes 失败")
		return
	}
	items := listFromJSON(mapValueFromJSON(result, "")["data"])
	modes := make([]codexAppCollaborationMode, 0, len(items))
	for _, item := range items {
		object := mapValue(item)
		mode := codexAppCollaborationMode{
			Name:                  stringValue(object["name"]),
			Mode:                  stringValue(object["mode"]),
			Model:                 stringValue(object["model"]),
			ReasoningEffort:       stringValue(object["reasoning_effort"]),
			DeveloperInstructions: stringValue(object["developer_instructions"]),
		}
		if mode.Name == "" && mode.Mode == "" {
			continue
		}
		modes = append(modes, mode)
	}
	r.mu.Lock()
	r.appCollaborationModes = modes
	r.mu.Unlock()
}

// refreshCodexAppSkills 使用 ctx 参数读取 Codex app-server skills 列表。
func (r *AgentRuntime) refreshCodexAppSkills(ctx context.Context) {
	requestCtx, cancel := context.WithTimeout(ctx, codexAppServerRequestTimeout)
	defer cancel()
	result, err := r.codexAppRequest(requestCtx, "skills/list", map[string]any{"cwd": []string{r.projectPath}})
	if err != nil {
		log.Ctx(ctx).Debug().Err(err).Str("chatID", r.chatID).Msg("读取 Codex skills 失败")
		return
	}
	skills := codexAppSkillsFromResult(result)
	r.mu.Lock()
	r.appSkills = skills
	r.mu.Unlock()
}

// loadCodexAppModels 使用 ctx 参数读取 Codex 模型和思考深度列表。
func (r *AgentRuntime) loadCodexAppModels(ctx context.Context) {
	result, err := r.codexAppRequest(ctx, "model/list", map[string]any{})
	if err != nil {
		log.Ctx(ctx).Debug().Err(err).Str("chatID", r.chatID).Msg("读取 Codex models 失败")
		return
	}
	models := codexAppModelsFromResult(result)
	if len(models) == 0 {
		return
	}
	r.mu.Lock()
	profile := r.profile
	profile.Models = mergeCodexDiscoveredModels(profile.Models, models, r.model)
	if normalized, err := normalizeAgentProfile(profile); err == nil {
		r.profile = normalized
	}
	r.mu.Unlock()
}

// codexAppThreadAlreadyLoaded 使用 ctx 和 sessionID 参数判断 app-server 是否已加载目标 thread。
func (r *AgentRuntime) codexAppThreadAlreadyLoaded(ctx context.Context, sessionID string) bool {
	result, err := r.codexAppRequest(ctx, "thread/loaded/list", map[string]any{})
	if err != nil {
		return false
	}
	data := listFromJSON(mapValueFromJSON(result, "")["data"])
	for _, item := range data {
		if stringValue(item) == sessionID {
			return true
		}
	}
	return false
}

// loadCodexAppThreadHistory 使用 ctx 和 sessionID 参数读取 Codex 原生 thread 历史。
func (r *AgentRuntime) loadCodexAppThreadHistory(ctx context.Context, sessionID string) {
	result, err := r.codexAppRequest(ctx, "thread/read", map[string]any{"threadId": sessionID})
	if err != nil {
		log.Ctx(ctx).Debug().Err(err).Str("chatID", r.chatID).Str("threadID", sessionID).Msg("读取 Codex thread 历史失败")
		return
	}
	messages := codexAppHistoryMessages(r.chatID, result)
	r.mu.Lock()
	r.appHistoryMessages = messages
	r.mu.Unlock()
}

// emitCodexAppDiscoveredState 使用 callbacks 参数报告已发现的运行时能力。
func (r *AgentRuntime) emitCodexAppDiscoveredState(callbacks AgentRunCallbacks) {
	r.mu.Lock()
	profile := cloneAgentProfile(r.profile)
	skills := cloneAgentSkillOptions(r.appSkills)
	history := cloneChatMessages(r.appHistoryMessages)
	r.appHistoryMessages = nil
	r.mu.Unlock()
	if callbacks.OnAgentProfile != nil && profile.ID != "" {
		callbacks.OnAgentProfile(profile)
	}
	if callbacks.OnAgentSkills != nil && len(skills) > 0 {
		callbacks.OnAgentSkills(skills)
	}
	if callbacks.OnHistory != nil && len(history) > 0 {
		callbacks.OnHistory(history)
	}
}

// codexAppInputItems 使用 prompt 参数构造 Codex app-server input 列表。
func (r *AgentRuntime) codexAppInputItems(prompt string) []map[string]any {
	if skillName, skillPath, args, ok := r.codexAppSlashSkill(prompt); ok {
		items := []map[string]any{{
			"type": "skill",
			"name": skillName,
			"path": skillPath,
		}}
		if strings.TrimSpace(args) != "" {
			items = append(items, map[string]any{
				"type":          "text",
				"text":          strings.TrimSpace(args),
				"text_elements": []any{},
			})
		}
		return items
	}
	if strings.TrimSpace(prompt) == "" {
		return []map[string]any{}
	}
	return []map[string]any{{
		"type":          "text",
		"text":          prompt,
		"text_elements": []any{},
	}}
}

// codexAppSlashSkill 使用 prompt 参数匹配结构化 slash skill 输入。
func (r *AgentRuntime) codexAppSlashSkill(prompt string) (string, string, string, bool) {
	trimmed := strings.TrimSpace(prompt)
	if !strings.HasPrefix(trimmed, "/") || len(trimmed) <= 1 {
		return "", "", "", false
	}
	withoutPrefix := strings.TrimPrefix(trimmed, "/")
	name, args, _ := strings.Cut(withoutPrefix, " ")
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") {
		return "", "", "", false
	}
	r.mu.Lock()
	skills := cloneAgentSkillOptions(r.appSkills)
	r.mu.Unlock()
	for _, skill := range skills {
		if skill.ID == name || skill.Label == name {
			return skill.ID, skill.Path, strings.TrimSpace(args), true
		}
	}
	return "", "", "", false
}

// codexAppCollaborationMode 使用 planMode 和 effort 参数返回 turn/start 协作模式参数。
func (r *AgentRuntime) codexAppCollaborationMode(planMode bool, effort string) map[string]any {
	mode := r.matchCodexAppCollaborationMode(planMode)
	if mode.Mode == "" && mode.Name == "" {
		if !planMode {
			return nil
		}
		settings := map[string]any{
			"model":                  r.model,
			"reasoning_effort":       nil,
			"developer_instructions": nil,
		}
		if effort != "" {
			settings["reasoning_effort"] = effort
		}
		return map[string]any{"mode": "plan", "settings": settings}
	}
	settings := map[string]any{}
	settings["model"] = firstNonEmpty(mode.Model, r.model)
	if reasoning := firstNonEmpty(effort, mode.ReasoningEffort); reasoning != "" {
		settings["reasoning_effort"] = reasoning
	}
	if mode.DeveloperInstructions != "" {
		settings["developer_instructions"] = mode.DeveloperInstructions
	}
	return map[string]any{
		"mode":     firstNonEmpty(mode.Mode, "code"),
		"settings": settings,
	}
}

// matchCodexAppCollaborationMode 使用 planMode 参数选择最合适的协作模式。
func (r *AgentRuntime) matchCodexAppCollaborationMode(planMode bool) codexAppCollaborationMode {
	r.mu.Lock()
	modes := append([]codexAppCollaborationMode(nil), r.appCollaborationModes...)
	r.mu.Unlock()
	for _, mode := range modes {
		name := strings.ToLower(mode.Name + " " + mode.Mode)
		if planMode && (strings.Contains(name, "plan") || strings.Contains(name, "read")) {
			return mode
		}
		if !planMode && (strings.Contains(name, "code") || strings.Contains(name, "auto")) {
			return mode
		}
	}
	if !planMode {
		for _, mode := range modes {
			name := strings.ToLower(mode.Name + " " + mode.Mode)
			if !strings.Contains(name, "plan") && !strings.Contains(name, "read") {
				return mode
			}
		}
	}
	if len(modes) > 0 {
		return modes[0]
	}
	return codexAppCollaborationMode{}
}

// startCodexAppTurn 使用 ctx、prompt、images 和 outputSchema 参数启动 Codex app-server 单轮运行。
func (r *AgentRuntime) startCodexAppTurn(ctx context.Context, prompt string, images []MessageImage, outputSchema map[string]any) error {
	imagePaths, err := r.prepareImageFiles(images)
	if err != nil {
		return err
	}
	r.refreshCodexAppSkills(ctx)
	inputItems := r.codexAppInputItems(prompt)
	for _, imagePath := range imagePaths {
		inputItems = append(inputItems, map[string]any{
			"type": "localImage",
			"path": imagePath,
		})
	}
	effort := codexAppReasoningEffort(r.reasoning)
	r.mu.Lock()
	planMode := r.planMode
	r.mu.Unlock()
	params := map[string]any{
		"threadId":       r.sessionID,
		"input":          inputItems,
		"cwd":            r.projectPath,
		"approvalPolicy": "never",
		"sandboxPolicy":  codexAppSandboxPolicy(),
		"model":          r.model,
	}
	if collaborationMode := r.codexAppCollaborationMode(planMode, effort); collaborationMode != nil {
		params["collaborationMode"] = collaborationMode
	}
	if effort != "" {
		params["effort"] = effort
	}
	if len(outputSchema) > 0 {
		params["outputSchema"] = outputSchema
	}
	requestCtx, cancel := context.WithTimeout(ctx, codexAppServerRequestTimeout)
	defer cancel()
	result, err := r.codexAppRequest(requestCtx, "turn/start", params)
	if err != nil {
		return err
	}
	if turnID := codexAppTurnID(result); turnID != "" {
		log.Ctx(ctx).Info().Str("chatID", r.chatID).Str("turnID", turnID).Bool("planMode", planMode).Msg("Codex app-server turn 已启动")
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
	case "item/commandExecution/requestApproval":
		if err := r.handleCodexAppApprovalRequest(message, "command_approval"); err != nil {
			_ = r.codexAppError(message.ID, -32602, err.Error())
		}
	case "item/fileChange/requestApproval":
		if err := r.handleCodexAppApprovalRequest(message, "file_approval"); err != nil {
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

// handleCodexAppApprovalRequest 使用 message 和 toolName 参数自动批准权限请求。
func (r *AgentRuntime) handleCodexAppApprovalRequest(message codexAppRPCMessage, toolName string) error {
	params := mapValueFromJSON(message.Params, "")
	itemID := firstNonEmpty(stringValue(params["itemId"]), stringValue(params["item_id"]), "approval-"+codexAppRPCIDKey(message.ID))
	tool := ToolCall{
		ID:     "approval-" + itemID,
		Name:   toolName,
		Status: ToolCallStatusComplete,
		Input:  codexAppApprovalInput(params),
		Output: "已自动批准",
	}
	r.mu.Lock()
	callbacks := r.callbacks
	r.mu.Unlock()
	if callbacks.OnToolCall != nil {
		callbacks.OnToolCall(tool)
	}
	return r.codexAppResponse(message.ID, map[string]any{"decision": "accept"})
}

// handleCodexAppNotification 使用 method 和 params 参数消费 Codex app-server 通知。
func (r *AgentRuntime) handleCodexAppNotification(method string, params json.RawMessage) {
	event := ClaudeOutputEvent{}
	switch method {
	case "item/agentMessage/delta", "item/plan/delta":
		event.Delta = stringFromJSON(params, "delta")
	case "item/reasoning/summaryTextDelta":
		if tool := r.codexAppReasoningDelta(params); tool.ID != "" {
			event.ToolCalls = append(event.ToolCalls, tool)
		}
	case "item/started", "item/completed", "item/updated":
		event = codexAppEventFromItem(method, params)
	case "item/commandExecution/outputDelta", "command/exec/outputDelta":
		if tool := codexAppCommandOutputDelta(params); tool.ID != "" {
			event.ToolCalls = append(event.ToolCalls, tool)
		}
	case "thread/tokenUsage/updated":
		if usage := codexAppUsageFromTokenUsage(params); usage != nil {
			event.Usage = usage
		}
	case "item/commandExecution/terminalInteraction", "codex/event/terminal_interaction":
		if tool := codexAppTerminalInteraction(method, params); tool.ID != "" {
			event.ToolCalls = append(event.ToolCalls, tool)
		}
	case "codex/event/patch_apply_begin":
		if tool := codexAppPatchApply(params, true); tool.ID != "" {
			event.ToolCalls = append(event.ToolCalls, tool)
		}
	case "codex/event/patch_apply_end":
		if tool := codexAppPatchApply(params, false); tool.ID != "" {
			event.ToolCalls = append(event.ToolCalls, tool)
		}
	case "item/fileChange/outputDelta":
		if tool := codexAppFileChangeOutputDelta(params); tool.ID != "" {
			event.ToolCalls = append(event.ToolCalls, tool)
		}
	case "turn/completed":
		event = codexAppTurnCompletedEvent(params)
	case "error":
		event.Error = firstNonEmpty(stringFromJSON(mapValueFromJSON(params, "error"), "message"), stringFromJSON(params, "message"), "Codex app-server 运行失败")
	}
	if event.Delta == "" &&
		event.AssistantText == "" &&
		len(event.ToolCalls) == 0 &&
		event.Usage == nil &&
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
	case "fileChange":
		status := ToolCallStatusRunning
		switch stringValue(item["status"]) {
		case "completed":
			status = ToolCallStatusComplete
		case "failed", "declined":
			status = ToolCallStatusError
		}
		event.ToolCalls = append(event.ToolCalls, ToolCall{
			ID:     firstNonEmpty(stringValue(item["id"]), "codex-file-change"),
			Name:   "file_change",
			Status: status,
			Input:  jsonString(firstNonNil(item["changes"], item["files"], item["path"])),
			Output: firstNonEmpty(stringValue(item["aggregatedOutput"]), stringValue(item["output"])),
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
	case "collabAgentToolCall", "subAgent", "sub_agent":
		status := ToolCallStatusRunning
		switch stringValue(item["status"]) {
		case "completed":
			status = ToolCallStatusComplete
		case "failed":
			status = ToolCallStatusError
		}
		event.ToolCalls = append(event.ToolCalls, ToolCall{
			ID:     firstNonEmpty(stringValue(item["id"]), "codex-sub-agent"),
			Name:   "sub_agent",
			Status: status,
			Input: firstNonEmpty(
				stringValue(item["prompt"]),
				stringValue(item["task"]),
				stringValue(item["description"]),
				jsonString(firstNonNil(item["input"], item["arguments"])),
			),
			Output: firstNonEmpty(stringValue(item["aggregatedOutput"]), stringValue(item["output"]), jsonString(item["result"])),
		})
	case "reasoning":
		if text := codexAppReasoningText(item); text != "" {
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

// codexAppReasoningDelta 使用 params 参数累加 reasoning 摘要增量。
func (r *AgentRuntime) codexAppReasoningDelta(params json.RawMessage) ToolCall {
	itemID := stringFromJSON(params, "itemId")
	delta := stringFromJSON(params, "delta")
	if itemID == "" || delta == "" {
		return ToolCall{}
	}
	r.mu.Lock()
	if r.appReasoningByItem == nil {
		r.appReasoningByItem = make(map[string]string)
	}
	text := r.appReasoningByItem[itemID] + delta
	r.appReasoningByItem[itemID] = text
	r.mu.Unlock()
	return ToolCall{
		ID:     itemID,
		Name:   "thinking",
		Status: ToolCallStatusRunning,
		Input:  text,
	}
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

// codexAppFileChangeOutputDelta 使用 params 参数提取文件变更输出增量。
func codexAppFileChangeOutputDelta(params json.RawMessage) ToolCall {
	itemID := stringFromJSON(params, "itemId")
	delta := firstNonEmpty(stringFromJSON(params, "delta"), stringFromJSON(params, "chunk"))
	if itemID == "" || delta == "" {
		return ToolCall{}
	}
	return ToolCall{
		ID:     itemID,
		Name:   "file_change",
		Status: ToolCallStatusRunning,
		Output: delta,
	}
}

// codexAppApprovalInput 使用 params 参数生成权限请求摘要。
func codexAppApprovalInput(params map[string]any) string {
	return jsonString(map[string]any{
		"command": firstNonEmpty(stringValue(params["command"]), stringValue(params["cmd"])),
		"cwd":     stringValue(params["cwd"]),
		"reason":  stringValue(params["reason"]),
	})
}

// codexAppTerminalInteraction 使用 method 和 params 参数提取终端交互事件。
func codexAppTerminalInteraction(method string, params json.RawMessage) ToolCall {
	payload := mapValueFromJSON(params, "")
	source := payload
	if method == "codex/event/terminal_interaction" {
		source = mapValue(payload["msg"])
	}
	callID := firstNonEmpty(stringValue(source["call_id"]), stringValue(source["callId"]), stringValue(payload["itemId"]), "terminal-"+codexAppRPCIDKey(firstNonNil(source["process_id"], payload["processId"])))
	if callID == "" {
		return ToolCall{}
	}
	return ToolCall{
		ID:     callID,
		Name:   "terminal",
		Status: ToolCallStatusComplete,
		Input: jsonString(map[string]any{
			"processId": firstNonEmpty(codexAppRPCIDKey(source["process_id"]), codexAppRPCIDKey(payload["processId"])),
			"stdin":     firstNonEmpty(stringValue(source["stdin"]), stringValue(payload["stdin"])),
		}),
	}
}

// codexAppPatchApply 使用 params 和 running 参数提取 apply_patch 事件。
func codexAppPatchApply(params json.RawMessage, running bool) ToolCall {
	payload := mapValueFromJSON(params, "")
	msg := mapValue(payload["msg"])
	callID := firstNonEmpty(stringValue(msg["call_id"]), stringValue(msg["callId"]), "codex-apply-patch")
	status := ToolCallStatusComplete
	if running {
		status = ToolCallStatusRunning
	} else if success, ok := msg["success"].(bool); ok && !success {
		status = ToolCallStatusError
	}
	return ToolCall{
		ID:     callID,
		Name:   "apply_patch",
		Status: status,
		Input:  jsonString(msg["changes"]),
		Output: firstNonEmpty(stringValue(msg["stdout"]), stringValue(msg["stderr"])),
	}
}

// codexAppUsageFromTokenUsage 使用 params 参数提取 Codex token 和上下文窗口用量。
func codexAppUsageFromTokenUsage(params json.RawMessage) *AgentUsage {
	tokenUsage := mapValueFromJSON(params, "tokenUsage")
	if tokenUsage == nil {
		return nil
	}
	last := mapValue(tokenUsage["last"])
	usage := AgentUsage{
		InputTokens:             firstPositiveInt(last["inputTokens"], last["input_tokens"]),
		CachedInputTokens:       firstPositiveInt(last["cachedInputTokens"], last["cached_input_tokens"]),
		OutputTokens:            firstPositiveInt(last["outputTokens"], last["output_tokens"]),
		ContextWindowMaxTokens:  firstPositiveInt(tokenUsage["model_context_window"], tokenUsage["modelContextWindow"]),
		ContextWindowUsedTokens: firstPositiveInt(last["total_tokens"], last["totalTokens"]),
	}
	if usage.InputTokens <= 0 &&
		usage.CachedInputTokens <= 0 &&
		usage.OutputTokens <= 0 &&
		usage.ContextWindowMaxTokens <= 0 &&
		usage.ContextWindowUsedTokens <= 0 {
		return nil
	}
	return &usage
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

// codexAppReasoningText 使用 item 参数提取 reasoning 摘要文本。
func codexAppReasoningText(item map[string]any) string {
	return firstNonEmpty(
		strings.Join(stringSliceValue(item["summary"]), "\n"),
		strings.Join(stringSliceValue(item["content"]), "\n"),
		stringValue(item["summary"]),
		stringValue(item["text"]),
		stringValue(item["content"]),
	)
}

// codexAppSkillsFromResult 使用 result 参数提取 app-server skills。
func codexAppSkillsFromResult(result json.RawMessage) []AgentSkillOption {
	root := mapValueFromJSON(result, "")
	entries := listFromJSON(root["data"])
	byID := make(map[string]AgentSkillOption)
	for _, entry := range entries {
		for _, item := range listFromJSON(mapValue(entry)["skills"]) {
			skill := mapValue(item)
			name := strings.TrimSpace(stringValue(skill["name"]))
			path := strings.TrimSpace(stringValue(skill["path"]))
			if name == "" || path == "" {
				continue
			}
			description := strings.TrimSpace(stringValue(skill["description"]))
			if description == "" {
				description = name
			}
			if _, exists := byID[name]; !exists {
				byID[name] = AgentSkillOption{
					ID:          name,
					Label:       name,
					Description: description,
					Path:        path,
				}
			}
		}
	}
	resultSkills := make([]AgentSkillOption, 0, len(byID))
	for _, skill := range byID {
		resultSkills = append(resultSkills, skill)
	}
	return resultSkills
}

// codexAppModelsFromResult 使用 result 参数提取模型和思考深度列表。
func codexAppModelsFromResult(result json.RawMessage) []AgentModelOption {
	root := mapValueFromJSON(result, "")
	items := listFromJSON(root["data"])
	models := make([]AgentModelOption, 0, len(items))
	for _, item := range items {
		raw := mapValue(item)
		id := firstNonEmpty(stringValue(raw["id"]), stringValue(raw["model"]))
		if id == "" {
			continue
		}
		models = append(models, AgentModelOption{
			ID:              id,
			Label:           id,
			Default:         boolValue(raw["isDefault"], raw["default"]),
			ReasoningLevels: codexAppReasoningLevels(raw),
		})
	}
	return models
}

// codexAppReasoningLevels 使用 model 参数提取模型支持的思考深度。
func codexAppReasoningLevels(model map[string]any) []AgentReasoningOption {
	defaultEffort := firstNonEmpty(stringValue(model["defaultReasoningEffort"]), stringValue(model["default_reasoning_effort"]))
	items := firstNonNil(model["supportedReasoningEfforts"], model["supported_reasoning_efforts"])
	values := listFromJSON(items)
	levels := make([]AgentReasoningOption, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		entry := mapValue(value)
		id := firstNonEmpty(
			stringValue(value),
			stringValue(entry["id"]),
			stringValue(entry["reasoningEffort"]),
			stringValue(entry["reasoning_effort"]),
		)
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		label := codexAppReasoningLabel(id)
		if rawLabel := strings.TrimSpace(stringValue(entry["label"])); rawLabel != "" {
			label = rawLabel
		}
		levels = append(levels, AgentReasoningOption{
			ID:          id,
			Label:       label,
			Description: strings.TrimSpace(stringValue(entry["description"])),
			Default:     id == defaultEffort,
		})
	}
	if len(levels) == 0 && defaultEffort != "" {
		levels = append(levels, AgentReasoningOption{
			ID:      defaultEffort,
			Label:   codexAppReasoningLabel(defaultEffort),
			Default: true,
		})
	}
	return normalizeReasoningLevels(levels)
}

// mergeCodexDiscoveredModels 使用 existing、discovered 和 selectedModel 参数合并模型列表。
func mergeCodexDiscoveredModels(existing []AgentModelOption, discovered []AgentModelOption, selectedModel string) []AgentModelOption {
	if len(discovered) == 0 {
		return cloneAgentModels(existing)
	}
	result := cloneAgentModels(discovered)
	selectedModel = strings.TrimSpace(selectedModel)
	if selectedModel != "" {
		found := false
		for _, model := range result {
			if model.ID == selectedModel {
				found = true
				break
			}
		}
		if !found {
			for _, model := range existing {
				if model.ID == selectedModel {
					result = append([]AgentModelOption{model}, result...)
					found = true
					break
				}
			}
			if !found {
				result = append([]AgentModelOption{{ID: selectedModel, Label: selectedModel}}, result...)
			}
		}
	}
	if !hasDefaultAgentModel(result) && len(result) > 0 {
		result[0].Default = true
	}
	return result
}

// codexAppHistoryMessages 使用 chatID 和 result 参数转换 Codex 原生历史。
func codexAppHistoryMessages(chatID string, result json.RawMessage) []ChatMessage {
	thread := mapValue(mapValueFromJSON(result, "")["thread"])
	turns := listFromJSON(thread["turns"])
	messages := make([]ChatMessage, 0, len(turns)*2)
	for _, turn := range turns {
		var assistant *ChatMessage
		for _, rawItem := range listFromJSON(mapValue(turn)["items"]) {
			item := mapValue(rawItem)
			itemType := normalizeCodexAppItemType(stringValue(item["type"]))
			switch itemType {
			case "userMessage":
				if assistant != nil {
					messages = append(messages, *assistant)
					assistant = nil
				}
				if text := extractCodexAppUserText(item["content"]); text != "" {
					messages = append(messages, newCodexAppHistoryTextMessage(chatID, MessageRoleUser, text))
				}
			case "agentMessage", "plan":
				if assistant == nil {
					message := newCodexAppHistoryTextMessage(chatID, MessageRoleAssistant, "")
					assistant = &message
				}
				text := stringValue(item["text"])
				if text != "" {
					assistant.Text += text
					assistant.Parts = append(assistant.Parts, MessagePart{
						ID:        newID("part"),
						Type:      MessagePartTypeText,
						Text:      text,
						CreatedAt: assistant.CreatedAt,
						UpdatedAt: assistant.UpdatedAt,
					})
				}
			case "reasoning", "commandExecution", "fileChange", "collabAgentToolCall", "subAgent", "sub_agent":
				if assistant == nil {
					message := newCodexAppHistoryTextMessage(chatID, MessageRoleAssistant, "")
					assistant = &message
				}
				if event := codexAppEventFromItem("item/completed", rawJSON(map[string]any{"item": rawItem})); len(event.ToolCalls) > 0 {
					for _, tool := range event.ToolCalls {
						now := assistant.UpdatedAt
						tool.CreatedAt = now
						tool.UpdatedAt = now
						assistant.ToolCalls = append(assistant.ToolCalls, tool)
						upsertMessageToolPart(assistant, tool, now)
					}
				}
			}
		}
		if assistant != nil && (assistant.Text != "" || len(assistant.ToolCalls) > 0 || len(assistant.Parts) > 0) {
			messages = append(messages, *assistant)
		}
	}
	return messages
}

// newCodexAppHistoryTextMessage 使用 chatID、role 和 text 参数构造历史消息。
func newCodexAppHistoryTextMessage(chatID string, role string, text string) ChatMessage {
	now := time.Now()
	message := ChatMessage{
		ID:        newID("msg"),
		ChatID:    chatID,
		Role:      role,
		Text:      text,
		Status:    MessageStatusComplete,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if role == MessageRoleAssistant && text != "" {
		message.Parts = []MessagePart{{
			ID:        newID("part"),
			Type:      MessagePartTypeText,
			Text:      text,
			CreatedAt: now,
			UpdatedAt: now,
		}}
	}
	return message
}

// extractCodexAppUserText 使用 value 参数提取用户历史文本。
func extractCodexAppUserText(value any) string {
	if text := stringValue(value); text != "" {
		return text
	}
	parts := listFromJSON(value)
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		item := mapValue(part)
		if text := firstNonEmpty(stringValue(item["text"]), stringValue(item["content"])); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "")
}

// normalizeCodexAppItemType 使用 itemType 参数归一化 Codex 历史 item 类型。
func normalizeCodexAppItemType(itemType string) string {
	switch itemType {
	case "user_message", "userMessage":
		return "userMessage"
	case "agent_message", "agentMessage":
		return "agentMessage"
	case "command_execution", "commandExecution":
		return "commandExecution"
	case "file_change", "fileChange":
		return "fileChange"
	case "collab_agent_tool_call", "collabAgentToolCall":
		return "collabAgentToolCall"
	default:
		return itemType
	}
}

// codexAppReasoningLabel 使用 id 参数返回思考深度展示名。
func codexAppReasoningLabel(id string) string {
	switch id {
	case "low":
		return "Low"
	case "medium":
		return "Medium"
	case "high":
		return "High"
	case "xhigh":
		return "Extra high"
	default:
		return id
	}
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

// firstPositiveInt 返回 values 参数中的第一个正整数。
func firstPositiveInt(values ...any) int {
	for _, value := range values {
		if number := intValue(value); number > 0 {
			return number
		}
	}
	return 0
}

// intValue 使用 value 参数转换整数。
func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if number, err := typed.Int64(); err == nil {
			return int(number)
		}
		if number, err := typed.Float64(); err == nil {
			return int(number)
		}
	}
	return 0
}

// boolValue 返回 values 中第一个布尔值。
func boolValue(values ...any) bool {
	for _, value := range values {
		if typed, ok := value.(bool); ok {
			return typed
		}
	}
	return false
}

// codexAppSandboxPolicy 返回允许 Codex 任意操作的沙箱策略。
func codexAppSandboxPolicy() map[string]any {
	return map[string]any{"type": "dangerFullAccess"}
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

// rawJSON 使用 value 参数生成 JSON 原始消息，失败时返回空对象。
func rawJSON(value any) json.RawMessage {
	data, ok := rawJSONValue(value)
	if !ok {
		return json.RawMessage(`{}`)
	}
	return data
}

// listFromJSON 使用 value 参数读取数组。
func listFromJSON(value any) []any {
	raw, ok := rawJSONValue(value)
	if !ok {
		return nil
	}
	var list []any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&list); err != nil {
		return nil
	}
	return list
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
