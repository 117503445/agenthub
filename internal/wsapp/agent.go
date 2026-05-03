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
	"time"

	"github.com/rs/zerolog/log"
)

// AgentConfig 表示启动外部 agent 子进程所需的配置。
type AgentConfig struct {
	Command              string                // Command 表示 Claude 命令，保留该字段兼容旧配置。
	CodexCommand         string                // CodexCommand 表示 Codex 命令。
	MockClaudeCommand    string                // MockClaudeCommand 表示 Mock Claude Code 命令。
	MockCodexCommand     string                // MockCodexCommand 表示 Mock Codex 命令。
	AnthropicBaseURL     string                // AnthropicBaseURL 表示 Anthropic 兼容接口地址。
	AnthropicModel       string                // AnthropicModel 表示 Claude 使用的模型。
	AnthropicAPIKey      string                // AnthropicAPIKey 表示 Anthropic API Key。
	OpenAIBaseURL        string                // OpenAIBaseURL 表示 OpenAI 兼容接口地址。
	OpenAIAPIKey         string                // OpenAIAPIKey 表示 OpenAI API Key。
	MockAnthropicBaseURL string                // MockAnthropicBaseURL 表示 Mock Claude 固定使用的后端 Anthropic 兼容接口地址。
	MockAnthropicAPIKey  string                // MockAnthropicAPIKey 表示 Mock Claude 使用的 API Key。
	MockOpenAIBaseURL    string                // MockOpenAIBaseURL 表示 Mock Codex 固定使用的后端 OpenAI 兼容接口地址。
	MockOpenAIAPIKey     string                // MockOpenAIAPIKey 表示 Mock Codex 使用的 API Key。
	AgentProviders       []AgentProviderOption // AgentProviders 表示可用 agent 和模型选项。
}

// AgentRunCallbacks 表示一次 agent 运行中的回调。
type AgentRunCallbacks struct {
	OnSessionID func(sessionID string) // OnSessionID 使用 sessionID 参数记录 Claude 会话标识。
	OnDelta     func(delta string)     // OnDelta 使用 delta 参数追加 assistant 流式文本。
	OnToolCall  func(tool ToolCall)    // OnToolCall 使用 tool 参数报告工具调用状态。
	OnDone      func()                 // OnDone 表示当前轮次完成。
	OnError     func(message string)   // OnError 使用 message 参数报告当前轮次失败。
}

// AgentRunInput 表示发送 prompt 到 agent 的参数。
type AgentRunInput struct {
	ChatID             string            // ChatID 表示聊天页标识。
	ProjectPath        string            // ProjectPath 表示 agent 子进程工作目录。
	Provider           string            // Provider 表示 agent provider。
	Model              string            // Model 表示 agent 模型。
	Reasoning          string            // Reasoning 表示 agent 推理级别。
	Prompt             string            // Prompt 表示用户输入。
	SessionID          string            // SessionID 表示已有 agent 会话标识。
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
	provider             string
	model                string
	reasoning            string
	projectPath          string
	sessionID            string
	cmd                  *exec.Cmd
	stdin                io.WriteCloser
	stderrDone           chan struct{}
	mu                   sync.Mutex
	running              bool
	stopping             bool
	stderrLines          []string
	currentMessageID     string
	emittedAssistantText string
	callbacks            AgentRunCallbacks
}

// NewAgentManager 使用 ctx 和 config 参数创建 AgentManager。
func NewAgentManager(ctx context.Context, config AgentConfig) *AgentManager {
	if strings.TrimSpace(config.Command) == "" {
		config.Command = "claude"
	}
	if strings.TrimSpace(config.CodexCommand) == "" {
		config.CodexCommand = "codex"
	}
	if strings.TrimSpace(config.MockClaudeCommand) == "" {
		config.MockClaudeCommand = config.Command
	}
	if strings.TrimSpace(config.MockCodexCommand) == "" {
		config.MockCodexCommand = config.CodexCommand
	}
	if strings.TrimSpace(config.AnthropicModel) == "" {
		config.AnthropicModel = DefaultAgentModel(AgentProviderClaudeCode, DefaultAgentProviderOptions())
	}
	if len(config.AgentProviders) == 0 {
		config.AgentProviders = AgentProviderOptions(AgentOptionsConfig{
			ClaudeDefaultModel: config.AnthropicModel,
			CodexDefaultEffort: "xhigh",
		})
	}
	return &AgentManager{
		ctx:      ctx,
		config:   config,
		runtimes: make(map[string]*AgentRuntime),
	}
}

// Send 使用 input 参数把 prompt 发送到聊天页对应的 agent runtime。
func (m *AgentManager) Send(ctx context.Context, input AgentRunInput) error {
	provider, model, reasoning, err := NormalizeAgentSelection(input.Provider, input.Model, input.Reasoning, m.agentProviders())
	if err != nil {
		return err
	}
	input.Provider = provider
	input.Model = model
	input.Reasoning = reasoning

	switch provider {
	case AgentProviderCodex, AgentProviderMockCodex:
		return m.sendCodex(ctx, input)
	default:
		return m.sendClaude(ctx, input)
	}
}

// SetAgentProviders 使用 options 参数更新 agent 可选项。
func (m *AgentManager) SetAgentProviders(options []AgentProviderOption) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.AgentProviders = cloneAgentProviderOptions(options)
}

// agentProviders 返回当前 agent 可选项副本。
func (m *AgentManager) agentProviders() []AgentProviderOption {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneAgentProviderOptions(m.config.AgentProviders)
}

// sendClaude 使用 input 参数把 prompt 发送到聊天页对应的 Claude runtime。
func (m *AgentManager) sendClaude(ctx context.Context, input AgentRunInput) error {
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

// sendCodex 使用 ctx 和 input 参数启动 Codex CLI 单轮运行。
func (m *AgentManager) sendCodex(ctx context.Context, input AgentRunInput) error {
	runtime, err := m.registerEphemeralRuntime(input)
	if err != nil {
		return err
	}
	if err := runtime.startCodex(ctx, input.Prompt); err != nil {
		runtime.failCurrentRun(err.Error())
		m.removeRuntime(input.ChatID, runtime)
		return err
	}
	return nil
}

// Stop 使用 chatID 参数停止聊天页当前 agent runtime。
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

// registerEphemeralRuntime 使用 input 参数注册一次性 runtime。
func (m *AgentManager) registerEphemeralRuntime(input AgentRunInput) (*AgentRuntime, error) {
	runtimeCtx, cancel := context.WithCancel(m.ctx)
	runtime := &AgentRuntime{
		manager:              m,
		ctx:                  runtimeCtx,
		cancel:               cancel,
		chatID:               input.ChatID,
		provider:             input.Provider,
		model:                input.Model,
		reasoning:            input.Reasoning,
		projectPath:          input.ProjectPath,
		sessionID:            input.SessionID,
		running:              true,
		currentMessageID:     input.AssistantMessageID,
		emittedAssistantText: "",
		callbacks:            input.Callbacks,
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.runtimes[input.ChatID]; existing != nil && existing.isAlive() {
		cancel()
		return nil, fmt.Errorf("chat %s 已经有运行中的 agent", input.ChatID)
	}
	m.runtimes[input.ChatID] = runtime
	return runtime, nil
}

// ensureRuntime 使用 ctx 和 input 参数获取或启动 Claude runtime。
func (m *AgentManager) ensureRuntime(ctx context.Context, input AgentRunInput) (*AgentRuntime, error) {
	var stale *AgentRuntime
	m.mu.Lock()
	existing := m.runtimes[input.ChatID]
	if existing != nil &&
		existing.provider == input.Provider &&
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
		manager:     m,
		ctx:         runtimeCtx,
		cancel:      cancel,
		chatID:      input.ChatID,
		provider:    input.Provider,
		model:       input.Model,
		reasoning:   input.Reasoning,
		projectPath: input.ProjectPath,
		sessionID:   input.SessionID,
	}
	m.runtimes[input.ChatID] = runtime
	m.mu.Unlock()

	if stale != nil {
		stale.stop()
	}
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
	if r.provider == AgentProviderCodex || r.provider == AgentProviderMockCodex {
		return r.running
	}
	return r.cmd != nil && r.cmd.Process != nil && r.cmd.ProcessState == nil && r.stdin != nil
}

// start 使用 ctx 参数启动 Claude 子进程。
func (r *AgentRuntime) start(ctx context.Context) error {
	args := r.claudeArgs()
	if strings.TrimSpace(r.sessionID) != "" {
		args = append(args, "--resume", r.sessionID)
	}

	command := r.manager.config.Command
	if r.provider == AgentProviderMockClaudeCode {
		command = r.manager.config.MockClaudeCommand
	}
	cmd := exec.CommandContext(r.ctx, command, args...)
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
	r.stderrDone = make(chan struct{})
	stderrDone := r.stderrDone
	r.mu.Unlock()

	log.Ctx(ctx).Info().
		Str("chatID", r.chatID).
		Str("command", command).
		Str("cwd", r.projectPath).
		Str("baseURL", r.anthropicBaseURL()).
		Str("model", r.model).
		Msg("Claude runtime 已启动")

	go r.scanStdout(stdout)
	go r.scanStderr(stderr, stderrDone)
	go r.wait()
	return nil
}

// claudeArgs 返回当前 Claude runtime 的启动参数。
func (r *AgentRuntime) claudeArgs() []string {
	args := []string{
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--replay-user-messages",
		"--verbose",
		"--model", r.model,
	}
	if r.provider == AgentProviderMockClaudeCode {
		return append([]string{"--bare", "--setting-sources", "local"}, args...)
	}
	return append([]string{"--dangerously-skip-permissions"}, args...)
}

// startCodex 使用 ctx 参数启动 Codex CLI 单轮运行。
func (r *AgentRuntime) startCodex(ctx context.Context, prompt string) error {
	args := []string{"exec"}
	resumeSessionID := strings.TrimSpace(r.sessionID)
	if resumeSessionID != "" {
		args = append(args, "resume")
	}
	if r.provider == AgentProviderMockCodex {
		args = append(args, r.mockCodexConfigArgs()...)
	}
	if strings.TrimSpace(r.reasoning) != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", r.reasoning))
	}
	args = append(args,
		"--json",
		"--dangerously-bypass-approvals-and-sandbox",
		"--skip-git-repo-check",
		"--model", r.model,
	)
	if resumeSessionID == "" {
		args = append(args, "--cd", r.projectPath)
	} else {
		args = append(args, resumeSessionID)
	}
	args = append(args, prompt)

	command := r.manager.config.CodexCommand
	if r.provider == AgentProviderMockCodex {
		command = r.manager.config.MockCodexCommand
	}
	cmd := exec.CommandContext(r.ctx, command, args...)
	cmd.Dir = r.projectPath
	cmd.Env = r.codexEnv()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 Codex stdout 失败: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("创建 Codex stderr 失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Codex 失败: %w", err)
	}

	r.mu.Lock()
	r.cmd = cmd
	r.stderrDone = make(chan struct{})
	stderrDone := r.stderrDone
	r.mu.Unlock()

	log.Ctx(ctx).Info().
		Str("chatID", r.chatID).
		Str("command", command).
		Str("cwd", r.projectPath).
		Str("baseURL", r.codexBaseURL()).
		Str("model", r.model).
		Str("reasoning", r.reasoning).
		Msg("Codex runtime 已启动")

	go r.scanStdout(stdout)
	go r.scanStderr(stderr, stderrDone)
	go r.wait()
	return nil
}

// runMock 使用 prompt 参数运行内置 mock agent。
func (r *AgentRuntime) runMock(prompt string) {
	sessionID := r.sessionID
	if sessionID == "" {
		sessionID = "mock-" + newID("session")
	}
	if r.callbacks.OnSessionID != nil {
		r.callbacks.OnSessionID(sessionID)
	}

	if r.model == "mock-tool" && r.callbacks.OnToolCall != nil {
		r.callbacks.OnToolCall(ToolCall{
			ID:     "mock-read-readme",
			Name:   "Read",
			Status: ToolCallStatusRunning,
			Input:  `{"file_path":"README.md"}`,
		})
	}

	text := fmt.Sprintf("Mock Claude 正在回复：%s\n\n这是来自后端 mock agent 的流式内容，用于验证 agent 选择、模型选择、工具调用展示和会话恢复。", strings.TrimSpace(prompt))
	chunks := splitMockAgentResponse(text)
	for _, chunk := range chunks {
		select {
		case <-r.ctx.Done():
			return
		case <-time.After(80 * time.Millisecond):
		}
		select {
		case <-r.ctx.Done():
			return
		default:
		}
		if r.callbacks.OnDelta != nil {
			r.callbacks.OnDelta(chunk)
		}
	}
	if r.model == "mock-tool" && r.callbacks.OnToolCall != nil {
		r.callbacks.OnToolCall(ToolCall{
			ID:     "mock-read-readme",
			Name:   "Read",
			Status: ToolCallStatusComplete,
			Input:  `{"file_path":"README.md"}`,
			Output: "已读取 README.md",
		})
	}

	r.mu.Lock()
	running := r.running
	r.running = false
	r.mu.Unlock()
	r.manager.removeRuntime(r.chatID, r)
	if running && r.callbacks.OnDone != nil {
		r.callbacks.OnDone()
	}
}

// splitMockAgentResponse 使用 text 参数拆分 mock agent 输出片段。
func splitMockAgentResponse(text string) []string {
	runes := []rune(text)
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
	env["ANTHROPIC_MODEL"] = r.model
	if baseURL := r.anthropicBaseURL(); baseURL != "" {
		env["ANTHROPIC_BASE_URL"] = baseURL
		env["CLAUDE_CODE_API_BASE_URL"] = baseURL
	}
	if apiKey := r.anthropicAPIKey(); apiKey != "" {
		env["ANTHROPIC_API_KEY"] = apiKey
	}
	if r.provider == AgentProviderMockClaudeCode {
		env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] = "1"
	}
	env["DISABLE_AUTOUPDATER"] = "1"

	result := make([]string, 0, len(env))
	for key, value := range env {
		result = append(result, key+"="+value)
	}
	return result
}

// codexEnv 返回当前 Codex runtime 子进程使用的环境变量。
func (r *AgentRuntime) codexEnv() []string {
	env := map[string]string{}
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			env[key] = value
		}
	}
	if r.provider == AgentProviderMockCodex {
		if baseURL := r.codexBaseURL(); baseURL != "" {
			env["OPENAI_BASE_URL"] = baseURL
		}
		if apiKey := r.codexAPIKey(); apiKey != "" {
			env["OPENAI_API_KEY"] = apiKey
		}
		if baseURL := r.mockAnthropicBaseURL(); baseURL != "" {
			env["ANTHROPIC_BASE_URL"] = baseURL
		}
		if apiKey := r.mockAnthropicAPIKey(); apiKey != "" {
			env["ANTHROPIC_API_KEY"] = apiKey
		}
	}

	result := make([]string, 0, len(env))
	for key, value := range env {
		result = append(result, key+"="+value)
	}
	return result
}

// mockCodexConfigArgs 返回 mock Codex 使用的真实 CLI 配置参数。
func (r *AgentRuntime) mockCodexConfigArgs() []string {
	baseURL := r.codexBaseURL()
	if baseURL == "" {
		return nil
	}
	providerConfig := fmt.Sprintf(
		`{name="Coding Mock OpenAI", base_url=%q, env_key="OPENAI_API_KEY", wire_api="responses"}`,
		baseURL,
	)
	return []string{
		"--ignore-user-config",
		"-c", `model_provider="coding_mock"`,
		"-c", "model_providers.coding_mock=" + providerConfig,
	}
}

// codexBaseURL 返回 mock Codex 使用的 OpenAI 兼容接口地址。
func (r *AgentRuntime) codexBaseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(r.manager.config.OpenAIBaseURL), "/")
	if r.provider == AgentProviderMockCodex {
		baseURL = strings.TrimRight(strings.TrimSpace(r.manager.config.MockOpenAIBaseURL), "/")
	}
	if baseURL != "" {
		return baseURL
	}
	anthropicBaseURL := strings.TrimRight(strings.TrimSpace(r.manager.config.AnthropicBaseURL), "/")
	if r.provider == AgentProviderMockCodex {
		anthropicBaseURL = r.mockAnthropicBaseURL()
	}
	if strings.HasSuffix(anthropicBaseURL, "/mock/anthropic") {
		return strings.TrimSuffix(anthropicBaseURL, "/mock/anthropic") + "/mock/openai/v1"
	}
	return ""
}

// anthropicBaseURL 返回当前 Claude runtime 使用的 Anthropic 兼容接口地址。
func (r *AgentRuntime) anthropicBaseURL() string {
	if r.provider == AgentProviderMockClaudeCode {
		return r.mockAnthropicBaseURL()
	}
	return strings.TrimRight(strings.TrimSpace(r.manager.config.AnthropicBaseURL), "/")
}

// anthropicAPIKey 返回当前 Claude runtime 使用的 API Key。
func (r *AgentRuntime) anthropicAPIKey() string {
	if r.provider == AgentProviderMockClaudeCode {
		return r.mockAnthropicAPIKey()
	}
	return strings.TrimSpace(r.manager.config.AnthropicAPIKey)
}

// mockAnthropicBaseURL 返回 Mock Claude 使用的后端 mock 地址。
func (r *AgentRuntime) mockAnthropicBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(r.manager.config.MockAnthropicBaseURL), "/")
}

// mockAnthropicAPIKey 返回 Mock Claude 使用的 API Key。
func (r *AgentRuntime) mockAnthropicAPIKey() string {
	return strings.TrimSpace(r.manager.config.MockAnthropicAPIKey)
}

// codexAPIKey 返回 Mock Codex 使用的 API Key。
func (r *AgentRuntime) codexAPIKey() string {
	apiKey := strings.TrimSpace(r.manager.config.OpenAIAPIKey)
	if r.provider == AgentProviderMockCodex {
		apiKey = strings.TrimSpace(r.manager.config.MockOpenAIAPIKey)
	}
	if apiKey == "" {
		apiKey = r.mockAnthropicAPIKey()
	}
	return apiKey
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

// scanStderr 使用 stderr 和 done 参数读取 agent 错误输出并记录日志。
func (r *AgentRuntime) scanStderr(stderr io.Reader, done chan struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 16*1024), 512*1024)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		r.appendStderrLine(text)
		log.Ctx(r.ctx).Error().Str("chatID", r.chatID).Str("stderr", text).Str("provider", r.provider).Msg("agent stderr")
	}
}

// appendStderrLine 使用 text 参数记录 stderr 摘要。
func (r *AgentRuntime) appendStderrLine(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stderrLines = append(r.stderrLines, text)
	if len(r.stderrLines) > 20 {
		r.stderrLines = r.stderrLines[len(r.stderrLines)-20:]
	}
}

// stderrText 返回当前 runtime 的 stderr 摘要。
func (r *AgentRuntime) stderrText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.stderrLines, "\n")
}

// wait 等待 agent 子进程退出，并在异常退出时回调当前轮次失败。
func (r *AgentRuntime) wait() {
	err := r.cmd.Wait()
	r.manager.removeRuntime(r.chatID, r)

	r.mu.Lock()
	running := r.running
	stopping := r.stopping
	callbacks := r.callbacks
	stderrDone := r.stderrDone
	r.running = false
	r.stdin = nil
	r.mu.Unlock()
	r.waitStderrDone(stderrDone)

	if !running || stopping {
		return
	}
	if err != nil {
		if callbacks.OnError != nil {
			message := fmt.Sprintf("%s 进程退出: %v", r.provider, err)
			if stderr := strings.TrimSpace(r.stderrText()); stderr != "" {
				message += "\n" + stderr
			}
			callbacks.OnError(message)
		}
		return
	}
	if callbacks.OnDone != nil {
		callbacks.OnDone()
	}
}

// waitStderrDone 使用 done 参数等待 stderr 读取结束。
func (r *AgentRuntime) waitStderrDone(done chan struct{}) {
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
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

// consumeLine 使用 line 参数解析 agent 输出并触发回调。
func (r *AgentRuntime) consumeLine(line []byte) {
	event, err := r.parseOutputLine(line)
	if err != nil {
		if r.provider == AgentProviderCodex || r.provider == AgentProviderMockCodex {
			text := strings.TrimSpace(string(line))
			if text != "" {
				event = ClaudeOutputEvent{Delta: text + "\n"}
			} else {
				return
			}
		} else {
			log.Ctx(r.ctx).Debug().Err(err).Str("chatID", r.chatID).Str("line", string(line)).Msg("忽略非 JSON agent 输出")
			return
		}
	}

	r.consumeOutputEvent(event)
}

// parseOutputLine 使用 line 参数按 provider 解析 agent 输出。
func (r *AgentRuntime) parseOutputLine(line []byte) (ClaudeOutputEvent, error) {
	if r.provider == AgentProviderCodex || r.provider == AgentProviderMockCodex {
		return parseCodexOutputLine(line)
	}
	return parseClaudeOutputLine(line)
}

// consumeOutputEvent 使用 event 参数触发当前轮次回调。
func (r *AgentRuntime) consumeOutputEvent(event ClaudeOutputEvent) {
	var onSessionID func(string)
	var onDelta func(string)
	var onToolCall func(ToolCall)
	var onDone func()
	var onError func(string)
	var sessionID string
	var delta string
	var toolCalls []ToolCall
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
		if len(event.ToolCalls) > 0 {
			toolCalls = append([]ToolCall(nil), event.ToolCalls...)
			onToolCall = r.callbacks.OnToolCall
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
	if onToolCall != nil {
		for _, tool := range toolCalls {
			onToolCall(tool)
		}
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
	SessionID     string     // SessionID 表示 Claude 会话标识。
	Delta         string     // Delta 表示增量 assistant 文本。
	AssistantText string     // AssistantText 表示当前 assistant 完整文本。
	ToolCalls     []ToolCall // ToolCalls 表示本行携带的工具调用更新。
	Done          bool       // Done 表示当前轮次完成。
	Error         string     // Error 表示当前轮次错误。
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
		event.ToolCalls = append(event.ToolCalls, extractStreamEventToolCalls(raw["event"])...)
	case "assistant":
		event.AssistantText = extractAssistantMessageText(raw["message"])
		event.ToolCalls = append(event.ToolCalls, extractClaudeMessageToolCalls(raw["message"])...)
	case "user":
		event.ToolCalls = append(event.ToolCalls, extractClaudeToolResults(raw["message"])...)
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

// parseCodexOutputLine 使用 line 参数解析 Codex JSONL 输出。
func parseCodexOutputLine(line []byte) (ClaudeOutputEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return ClaudeOutputEvent{}, err
	}

	event := ClaudeOutputEvent{
		SessionID: firstNonEmpty(
			stringValue(raw["thread_id"]),
			stringValue(raw["threadId"]),
			stringValue(raw["session_id"]),
			stringValue(raw["sessionId"]),
		),
	}
	method := firstNonEmpty(stringValue(raw["method"]), stringValue(raw["type"]), stringValue(raw["msg"]))
	params := mapValue(raw["params"])
	if params == nil {
		params = mapValue(raw["item"])
	}
	if params == nil {
		params = raw
	}

	if method == "item.started" || method == "item.completed" || method == "item.updated" {
		itemType := stringValue(params["type"])
		switch itemType {
		case "agent_message":
			event.AssistantText = firstNonEmpty(stringValue(params["text"]), stringValue(params["message"]))
		case "command_execution":
			tool := ToolCall{
				ID:     firstNonEmpty(stringValue(params["id"]), "codex-command"),
				Name:   "exec_command",
				Status: ToolCallStatusRunning,
				Input:  stringValue(params["command"]),
				Output: firstNonEmpty(stringValue(params["aggregated_output"]), stringValue(params["output"])),
			}
			if method == "item.completed" || stringValue(params["status"]) == "completed" {
				tool.Status = ToolCallStatusComplete
			}
			if stringValue(params["status"]) == "failed" {
				tool.Status = ToolCallStatusError
			}
			event.ToolCalls = append(event.ToolCalls, tool)
		}
	}

	if strings.Contains(method, "agentMessage") || strings.Contains(method, "output_text.delta") {
		event.Delta = firstNonEmpty(stringValue(params["delta"]), stringValue(params["text"]), stringValue(params["content"]))
	}
	if event.Delta == "" && (method == "agent_message" || method == "assistant_message" || method == "message") {
		event.Delta = firstNonEmpty(stringValue(params["delta"]), stringValue(params["text"]), stringValue(params["message"]))
	}
	if strings.Contains(method, "reasoning") {
		if text := firstNonEmpty(stringValue(params["delta"]), stringValue(params["text"]), stringValue(params["summary"])); text != "" {
			event.ToolCalls = append(event.ToolCalls, ToolCall{
				ID:     firstNonEmpty(stringValue(params["itemId"]), "codex-reasoning"),
				Name:   "thinking",
				Status: ToolCallStatusRunning,
				Input:  text,
			})
		}
	}
	if strings.Contains(method, "commandExecution") || strings.Contains(method, "exec_command") {
		tool := ToolCall{
			ID:     firstNonEmpty(stringValue(params["itemId"]), stringValue(params["toolCallId"]), stringValue(params["callId"]), "codex-command"),
			Name:   "exec_command",
			Status: ToolCallStatusRunning,
			Input:  firstNonEmpty(stringValue(params["command"]), stringValue(params["cmd"]), jsonString(params["arguments"]), jsonString(params["input"])),
			Output: firstNonEmpty(stringValue(params["delta"]), stringValue(params["output"]), stringValue(params["content"])),
		}
		if strings.Contains(method, "completed") || strings.Contains(method, "end") {
			tool.Status = ToolCallStatusComplete
		}
		event.ToolCalls = append(event.ToolCalls, tool)
	}
	if strings.Contains(method, "tool/call") || strings.Contains(method, "mcpToolCall") || stringValue(params["toolName"]) != "" {
		tool := ToolCall{
			ID:     firstNonEmpty(stringValue(params["itemId"]), stringValue(params["toolCallId"]), stringValue(params["callId"]), "codex-tool"),
			Name:   firstNonEmpty(stringValue(params["toolName"]), stringValue(params["name"]), "tool"),
			Status: ToolCallStatusRunning,
			Input:  firstNonEmpty(jsonString(params["arguments"]), jsonString(params["input"])),
			Output: firstNonEmpty(stringValue(params["delta"]), stringValue(params["output"]), stringValue(params["result"])),
		}
		if strings.Contains(method, "completed") || strings.Contains(method, "end") {
			tool.Status = ToolCallStatusComplete
		}
		event.ToolCalls = append(event.ToolCalls, tool)
	}
	if strings.Contains(method, "turn/completed") || strings.Contains(method, "turn_completed") || method == "turn.completed" || method == "done" {
		event.Done = true
	}
	if errText := firstNonEmpty(codexErrorText(raw["error"]), codexErrorText(params["error"]), stringValue(raw["message"]), stringValue(params["message"])); errText != "" {
		event.Error = errText
	}
	return event, nil
}

// codexErrorText 使用 value 参数提取 Codex 错误文本。
func codexErrorText(value any) string {
	if text := stringValue(value); text != "" {
		return text
	}
	if block := mapValue(value); block != nil {
		return firstNonEmpty(stringValue(block["message"]), stringValue(block["error"]))
	}
	return ""
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

// extractStreamEventToolCalls 使用 value 参数提取 Claude stream_event 工具调用。
func extractStreamEventToolCalls(value any) []ToolCall {
	event, ok := value.(map[string]any)
	if !ok || stringValue(event["type"]) != "content_block_start" {
		return nil
	}
	block, ok := event["content_block"].(map[string]any)
	if !ok || stringValue(block["type"]) != "tool_use" {
		return nil
	}
	return []ToolCall{{
		ID:     firstNonEmpty(stringValue(block["id"]), newID("tool")),
		Name:   firstNonEmpty(stringValue(block["name"]), "tool"),
		Status: ToolCallStatusRunning,
		Input:  jsonString(block["input"]),
	}}
}

// extractAssistantMessageText 使用 value 参数提取 assistant 消息完整文本。
func extractAssistantMessageText(value any) string {
	message, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return collectClaudeContentText(message["content"])
}

// extractClaudeMessageToolCalls 使用 value 参数提取 Claude assistant 工具调用。
func extractClaudeMessageToolCalls(value any) []ToolCall {
	message, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	content, ok := message["content"].([]any)
	if !ok {
		return nil
	}
	tools := make([]ToolCall, 0)
	for _, item := range content {
		block, ok := item.(map[string]any)
		if !ok || stringValue(block["type"]) != "tool_use" {
			continue
		}
		tools = append(tools, ToolCall{
			ID:     firstNonEmpty(stringValue(block["id"]), newID("tool")),
			Name:   firstNonEmpty(stringValue(block["name"]), "tool"),
			Status: ToolCallStatusRunning,
			Input:  jsonString(block["input"]),
		})
	}
	return tools
}

// extractClaudeToolResults 使用 value 参数提取 Claude user 工具结果。
func extractClaudeToolResults(value any) []ToolCall {
	message, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	content, ok := message["content"].([]any)
	if !ok {
		return nil
	}
	tools := make([]ToolCall, 0)
	for _, item := range content {
		block, ok := item.(map[string]any)
		if !ok || stringValue(block["type"]) != "tool_result" {
			continue
		}
		tools = append(tools, ToolCall{
			ID:     firstNonEmpty(stringValue(block["tool_use_id"]), newID("tool")),
			Status: ToolCallStatusComplete,
			Output: collectClaudeContentText(block["content"]),
		})
	}
	return tools
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

// mapValue 使用 value 参数读取 map[string]any。
func mapValue(value any) map[string]any {
	result, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return result
}

// jsonString 使用 value 参数编码简短 JSON 字符串。
func jsonString(value any) string {
	if value == nil {
		return ""
	}
	if text := stringValue(value); text != "" {
		return text
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
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
