package wsapp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// AgentConfig 表示启动 Claude 子进程所需的配置。
type AgentConfig struct {
	Command          string // Command 表示 Claude 命令。
	AnthropicBaseURL string // AnthropicBaseURL 表示 Anthropic 兼容接口地址。
	AnthropicModel   string // AnthropicModel 表示 Claude 使用的模型。
	AnthropicAPIKey  string // AnthropicAPIKey 表示 Anthropic API Key。
}

// AgentRunCallbacks 表示一次 agent 运行中的回调。
type AgentRunCallbacks struct {
	OnSessionID func(sessionID string) // OnSessionID 使用 sessionID 参数记录 Claude 会话标识。
	OnDelta     func(delta string)     // OnDelta 使用 delta 参数追加 assistant 流式文本。
	OnDone      func()                 // OnDone 表示当前轮次完成。
	OnError     func(message string)   // OnError 使用 message 参数报告当前轮次失败。
}

// AgentRunInput 表示发送 prompt 到 agent 的参数。
type AgentRunInput struct {
	ChatID             string            // ChatID 表示聊天页标识。
	ProjectPath        string            // ProjectPath 表示 Claude 子进程工作目录。
	Prompt             string            // Prompt 表示用户输入。
	SessionID          string            // SessionID 表示已有 Claude 会话标识。
	AssistantMessageID string            // AssistantMessageID 表示本轮 assistant 消息标识。
	Callbacks          AgentRunCallbacks // Callbacks 表示运行过程回调。
}

// AgentManager 管理所有聊天页对应的 Claude runtime。
type AgentManager struct {
	ctx      context.Context
	config   AgentConfig
	mu       sync.Mutex
	runtimes map[string]*AgentRuntime
}

// AgentRuntime 表示一个聊天页中的 Claude 子进程。
type AgentRuntime struct {
	manager              *AgentManager
	ctx                  context.Context
	cancel               context.CancelFunc
	chatID               string
	projectPath          string
	sessionID            string
	cmd                  *exec.Cmd
	stdin                io.WriteCloser
	mu                   sync.Mutex
	running              bool
	stopping             bool
	currentMessageID     string
	emittedAssistantText string
	callbacks            AgentRunCallbacks
}

// NewAgentManager 使用 ctx 和 config 参数创建 AgentManager。
func NewAgentManager(ctx context.Context, config AgentConfig) *AgentManager {
	if strings.TrimSpace(config.Command) == "" {
		config.Command = "claude"
	}
	if strings.TrimSpace(config.AnthropicModel) == "" {
		config.AnthropicModel = "sonnet"
	}
	if strings.TrimSpace(config.AnthropicAPIKey) == "" {
		config.AnthropicAPIKey = "mock-key"
	}
	return &AgentManager{
		ctx:      ctx,
		config:   config,
		runtimes: make(map[string]*AgentRuntime),
	}
}

// Send 使用 input 参数把 prompt 发送到聊天页对应的 Claude runtime。
func (m *AgentManager) Send(ctx context.Context, input AgentRunInput) error {
	runtime, err := m.ensureRuntime(ctx, input)
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
	stdin := runtime.stdin
	runtime.mu.Unlock()

	line, err := buildClaudeUserMessage(input.Prompt, sessionID)
	if err != nil {
		runtime.failCurrentRun(err.Error())
		return err
	}
	if _, err := io.WriteString(stdin, line+"\n"); err != nil {
		runtime.failCurrentRun(err.Error())
		m.removeRuntime(input.ChatID, runtime)
		return err
	}
	log.Ctx(ctx).Info().
		Str("chatID", input.ChatID).
		Str("messageID", input.AssistantMessageID).
		Msg("已发送 prompt 到 Claude")
	return nil
}

// Stop 使用 chatID 参数停止聊天页当前 Claude runtime。
func (m *AgentManager) Stop(chatID string) bool {
	m.mu.Lock()
	runtime, ok := m.runtimes[chatID]
	if ok {
		delete(m.runtimes, chatID)
	}
	m.mu.Unlock()
	if !ok {
		return false
	}
	runtime.stop()
	return true
}

// ensureRuntime 使用 ctx 和 input 参数获取或启动 Claude runtime。
func (m *AgentManager) ensureRuntime(ctx context.Context, input AgentRunInput) (*AgentRuntime, error) {
	m.mu.Lock()
	existing := m.runtimes[input.ChatID]
	if existing != nil && existing.isAlive() {
		m.mu.Unlock()
		return existing, nil
	}
	if existing != nil {
		delete(m.runtimes, input.ChatID)
	}

	runtimeCtx, cancel := context.WithCancel(m.ctx)
	runtime := &AgentRuntime{
		manager:     m,
		ctx:         runtimeCtx,
		cancel:      cancel,
		chatID:      input.ChatID,
		projectPath: input.ProjectPath,
		sessionID:   input.SessionID,
	}
	m.runtimes[input.ChatID] = runtime
	m.mu.Unlock()

	if err := runtime.start(ctx); err != nil {
		cancel()
		m.removeRuntime(input.ChatID, runtime)
		return nil, err
	}
	return runtime, nil
}

// removeRuntime 使用 chatID 和 runtime 参数从管理器移除匹配的 runtime。
func (m *AgentManager) removeRuntime(chatID string, runtime *AgentRuntime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current := m.runtimes[chatID]; current == runtime {
		delete(m.runtimes, chatID)
	}
}

// isAlive 判断当前 runtime 子进程是否仍可用。
func (r *AgentRuntime) isAlive() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd != nil && r.cmd.Process != nil && r.cmd.ProcessState == nil && r.stdin != nil
}

// start 使用 ctx 参数启动 Claude 子进程。
func (r *AgentRuntime) start(ctx context.Context) error {
	args := []string{
		"--bare",
		"--setting-sources", "local",
		"--dangerously-skip-permissions",
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--replay-user-messages",
		"--verbose",
		"--model", r.manager.config.AnthropicModel,
	}
	if strings.TrimSpace(r.sessionID) != "" {
		args = append(args, "--resume", r.sessionID)
	}

	cmd := exec.CommandContext(r.ctx, r.manager.config.Command, args...)
	cmd.Dir = r.projectPath
	cmd.Env = r.childEnv()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("创建 Claude stdin 失败: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("创建 Claude stdout 失败: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("创建 Claude stderr 失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("启动 Claude 失败: %w", err)
	}

	r.mu.Lock()
	r.cmd = cmd
	r.stdin = stdin
	r.mu.Unlock()

	log.Ctx(ctx).Info().
		Str("chatID", r.chatID).
		Str("cwd", r.projectPath).
		Str("baseURL", r.manager.config.AnthropicBaseURL).
		Str("model", r.manager.config.AnthropicModel).
		Msg("Claude runtime 已启动")

	go r.scanStdout(stdout)
	go r.scanStderr(stderr)
	go r.wait()
	return nil
}

// childEnv 返回当前 runtime 子进程使用的环境变量。
func (r *AgentRuntime) childEnv() []string {
	env := map[string]string{}
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			env[key] = value
		}
	}
	for _, key := range []string{
		"CLAUDECODE",
		"CLAUDE_CODE_ENTRYPOINT",
		"CLAUDE_CODE_SSE_PORT",
		"CLAUDE_AGENT_SDK_VERSION",
		"CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING",
	} {
		delete(env, key)
	}
	env["IS_SANDBOX"] = "1"
	env["ANTHROPIC_BASE_URL"] = r.manager.config.AnthropicBaseURL
	env["CLAUDE_CODE_API_BASE_URL"] = r.manager.config.AnthropicBaseURL
	env["ANTHROPIC_MODEL"] = r.manager.config.AnthropicModel
	env["ANTHROPIC_API_KEY"] = r.manager.config.AnthropicAPIKey
	env["DISABLE_AUTOUPDATER"] = "1"

	result := make([]string, 0, len(env))
	for key, value := range env {
		result = append(result, key+"="+value)
	}
	return result
}

// scanStdout 使用 stdout 参数读取并解析 Claude JSON 行输出。
func (r *AgentRuntime) scanStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		r.consumeLine(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		log.Ctx(r.ctx).Warn().Err(err).Str("chatID", r.chatID).Msg("读取 Claude stdout 失败")
	}
}

// scanStderr 使用 stderr 参数读取 Claude 错误输出并记录日志。
func (r *AgentRuntime) scanStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 16*1024), 512*1024)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		log.Ctx(r.ctx).Error().Str("chatID", r.chatID).Str("stderr", text).Msg("Claude stderr")
	}
}

// wait 等待 Claude 子进程退出，并在异常退出时回调当前轮次失败。
func (r *AgentRuntime) wait() {
	err := r.cmd.Wait()
	r.manager.removeRuntime(r.chatID, r)

	r.mu.Lock()
	running := r.running
	stopping := r.stopping
	callbacks := r.callbacks
	r.running = false
	r.stdin = nil
	r.mu.Unlock()

	if !running || stopping {
		return
	}
	if err != nil {
		if callbacks.OnError != nil {
			callbacks.OnError(fmt.Sprintf("Claude 进程退出: %v", err))
		}
		return
	}
	if callbacks.OnDone != nil {
		callbacks.OnDone()
	}
}

// stop 停止当前 runtime 子进程。
func (r *AgentRuntime) stop() {
	r.mu.Lock()
	r.stopping = true
	r.running = false
	cancel := r.cancel
	cmd := r.cmd
	stdin := r.stdin
	r.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// failCurrentRun 使用 message 参数结束当前轮次并报告失败。
func (r *AgentRuntime) failCurrentRun(message string) {
	r.mu.Lock()
	callbacks := r.callbacks
	r.running = false
	r.mu.Unlock()
	if callbacks.OnError != nil {
		callbacks.OnError(message)
	}
}

// consumeLine 使用 line 参数解析 Claude 输出并触发回调。
func (r *AgentRuntime) consumeLine(line []byte) {
	event, err := parseClaudeOutputLine(line)
	if err != nil {
		log.Ctx(r.ctx).Debug().Err(err).Str("chatID", r.chatID).Str("line", string(line)).Msg("忽略非 JSON Claude 输出")
		return
	}

	var onSessionID func(string)
	var onDelta func(string)
	var onDone func()
	var onError func(string)
	var sessionID string
	var delta string
	var done bool
	var errorMessage string

	r.mu.Lock()
	if event.SessionID != "" && r.sessionID != event.SessionID {
		r.sessionID = event.SessionID
		onSessionID = r.callbacks.OnSessionID
		sessionID = event.SessionID
	}
	if r.running {
		if event.Delta != "" {
			r.emittedAssistantText += event.Delta
			onDelta = r.callbacks.OnDelta
			delta = event.Delta
		}
		if event.AssistantText != "" {
			if strings.HasPrefix(event.AssistantText, r.emittedAssistantText) {
				delta = event.AssistantText[len(r.emittedAssistantText):]
			} else if event.AssistantText != r.emittedAssistantText {
				delta = event.AssistantText
			}
			if delta != "" {
				r.emittedAssistantText = event.AssistantText
				onDelta = r.callbacks.OnDelta
			}
		}
		if event.Done {
			r.running = false
			done = true
			onDone = r.callbacks.OnDone
		}
		if event.Error != "" {
			r.running = false
			errorMessage = event.Error
			onError = r.callbacks.OnError
		}
	}
	r.mu.Unlock()

	if onSessionID != nil {
		onSessionID(sessionID)
	}
	if onDelta != nil && delta != "" {
		onDelta(delta)
	}
	if onError != nil && errorMessage != "" {
		onError(errorMessage)
		return
	}
	if onDone != nil && done {
		onDone()
	}
}

// ClaudeOutputEvent 表示从 Claude JSON 行中提取出的 UI 事件。
type ClaudeOutputEvent struct {
	SessionID     string // SessionID 表示 Claude 会话标识。
	Delta         string // Delta 表示增量 assistant 文本。
	AssistantText string // AssistantText 表示当前 assistant 完整文本。
	Done          bool   // Done 表示当前轮次完成。
	Error         string // Error 表示当前轮次错误。
}

// parseClaudeOutputLine 使用 line 参数解析 Claude JSON 行。
func parseClaudeOutputLine(line []byte) (ClaudeOutputEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return ClaudeOutputEvent{}, err
	}

	event := ClaudeOutputEvent{
		SessionID: firstString(raw, "session_id", "sessionId"),
	}
	messageType := stringValue(raw["type"])
	switch messageType {
	case "stream_event":
		event.Delta = extractStreamEventDelta(raw["event"])
	case "assistant":
		event.AssistantText = extractAssistantMessageText(raw["message"])
	case "result":
		if resultText := stringValue(raw["result"]); resultText != "" {
			event.AssistantText = resultText
		}
		if subtype := stringValue(raw["subtype"]); subtype == "error" {
			event.Error = firstNonEmpty(stringValue(raw["error"]), stringValue(raw["message"]), "Claude 运行失败")
		} else {
			event.Done = true
		}
	}
	if errorsValue, ok := raw["errors"].([]any); ok && len(errorsValue) > 0 {
		parts := make([]string, 0, len(errorsValue))
		for _, item := range errorsValue {
			if text := stringValue(item); text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			event.Error = strings.Join(parts, "\n")
		}
	}
	return event, nil
}

// extractStreamEventDelta 使用 value 参数提取 Claude stream_event 文本增量。
func extractStreamEventDelta(value any) string {
	event, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	switch stringValue(event["type"]) {
	case "content_block_start":
		block, ok := event["content_block"].(map[string]any)
		if !ok || stringValue(block["type"]) != "text" {
			return ""
		}
		return stringValue(block["text"])
	case "content_block_delta":
		delta, ok := event["delta"].(map[string]any)
		if !ok {
			return ""
		}
		if deltaType := stringValue(delta["type"]); deltaType != "text_delta" && deltaType != "input_json_delta" {
			return ""
		}
		return firstNonEmpty(stringValue(delta["text"]), stringValue(delta["partial_json"]))
	default:
		return ""
	}
}

// extractAssistantMessageText 使用 value 参数提取 assistant 消息完整文本。
func extractAssistantMessageText(value any) string {
	message, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return collectClaudeContentText(message["content"])
}

// collectClaudeContentText 使用 value 参数拼接 Claude content 文本。
func collectClaudeContentText(value any) string {
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
			if text := stringValue(block["text"]); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// buildClaudeUserMessage 使用 prompt 和 sessionID 参数构造 Claude stream-json 输入行。
func buildClaudeUserMessage(prompt string, sessionID string) (string, error) {
	payload := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]string{
				{
					"type": "text",
					"text": prompt,
				},
			},
		},
		"parent_tool_use_id": nil,
		"uuid":               newUUID(),
		"session_id":         sessionID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// newUUID 生成符合 UUID v4 形式的随机标识。
func newUUID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return newID("uuid")
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	hexValue := hex.EncodeToString(bytes[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexValue[0:8], hexValue[8:12], hexValue[12:16], hexValue[16:20], hexValue[20:32])
}

// firstString 使用 value、primary 和 secondary 参数读取第一个字符串字段。
func firstString(value map[string]any, primary string, secondary string) string {
	if text := stringValue(value[primary]); text != "" {
		return text
	}
	return stringValue(value[secondary])
}

// firstNonEmpty 返回 values 参数中的第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// stringValue 使用 value 参数转换字符串值。
func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}
