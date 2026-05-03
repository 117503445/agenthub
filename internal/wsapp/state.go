package wsapp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// ChatStatusIdle 表示聊天页当前没有 agent 输出。
	ChatStatusIdle = "idle"
	// ChatStatusRunning 表示聊天页当前有 agent 正在输出。
	ChatStatusRunning = "running"
	// ChatStatusError 表示聊天页最近一次运行失败。
	ChatStatusError = "error"
)

const (
	// MessageRoleUser 表示用户消息。
	MessageRoleUser = "user"
	// MessageRoleAssistant 表示 agent 输出消息。
	MessageRoleAssistant = "assistant"
	// MessageRoleSystem 表示系统提示消息。
	MessageRoleSystem = "system"
)

const (
	// AgentProviderClaudeCode 表示 Claude Code CLI agent。
	AgentProviderClaudeCode = "claude-code"
	// AgentProviderCodex 表示 Codex CLI agent。
	AgentProviderCodex = "codex"
	// AgentProviderMockClaudeCode 表示连接 mock 模型服务的 Claude Code agent。
	AgentProviderMockClaudeCode = "mock-claude-code"
	// AgentProviderMockCodex 表示连接 mock 模型服务的 Codex agent。
	AgentProviderMockCodex = "mock-codex"
)

const (
	// MessageStatusComplete 表示消息已经完成。
	MessageStatusComplete = "complete"
	// MessageStatusStreaming 表示消息正在流式输出。
	MessageStatusStreaming = "streaming"
	// MessageStatusStopped 表示消息被用户停止。
	MessageStatusStopped = "stopped"
	// MessageStatusError 表示消息输出失败。
	MessageStatusError = "error"
)

const (
	// ToolCallStatusRunning 表示工具调用正在执行。
	ToolCallStatusRunning = "running"
	// ToolCallStatusComplete 表示工具调用已经完成。
	ToolCallStatusComplete = "complete"
	// ToolCallStatusError 表示工具调用失败。
	ToolCallStatusError = "error"
)

const (
	// MessagePartTypeText 表示 assistant 文本片段。
	MessagePartTypeText = "text"
	// MessagePartTypeToolCall 表示 assistant 工具调用片段。
	MessagePartTypeToolCall = "tool_call"
)

var (
	// ErrNotFound 表示目标资源不存在。
	ErrNotFound = errors.New("资源不存在")
	// ErrInvalidInput 表示请求参数不合法。
	ErrInvalidInput = errors.New("请求参数不合法")
)

// Project 表示一个后端本机工作目录。
type Project struct {
	ID        string    `json:"id"`        // ID 表示 project 唯一标识。
	Name      string    `json:"name"`      // Name 表示由工作目录最后一级派生的展示名称。
	Path      string    `json:"path"`      // Path 表示 project 工作目录。
	Git       GitInfo   `json:"git"`       // Git 表示 project 当前 Git 摘要。
	CreatedAt time.Time `json:"createdAt"` // CreatedAt 表示创建时间。
	UpdatedAt time.Time `json:"updatedAt"` // UpdatedAt 表示更新时间。
}

// GitInfo 表示 project 工作目录的 Git 摘要。
type GitInfo struct {
	IsRepo bool   `json:"isRepo"` // IsRepo 表示当前目录是否位于 Git 仓库中。
	Branch string `json:"branch"` // Branch 表示当前分支或 HEAD 状态。
	Commit string `json:"commit"` // Commit 表示当前短提交哈希。
	Dirty  bool   `json:"dirty"`  // Dirty 表示工作区是否有未提交内容。
}

// ToolCall 表示 assistant 消息中的一次工具调用。
type ToolCall struct {
	ID        string    `json:"id"`               // ID 表示工具调用唯一标识。
	Name      string    `json:"name"`             // Name 表示工具名称。
	Status    string    `json:"status"`           // Status 表示工具调用状态。
	Input     string    `json:"input,omitempty"`  // Input 表示工具入参摘要。
	Output    string    `json:"output,omitempty"` // Output 表示工具输出摘要。
	CreatedAt time.Time `json:"createdAt"`        // CreatedAt 表示创建时间。
	UpdatedAt time.Time `json:"updatedAt"`        // UpdatedAt 表示更新时间。
}

// ContextWindowUsage 表示聊天页上下文窗口使用情况。
type ContextWindowUsage struct {
	MaxTokens  int `json:"maxTokens"`  // MaxTokens 表示当前模型上下文窗口上限。
	UsedTokens int `json:"usedTokens"` // UsedTokens 表示当前聊天估算或 agent 上报的已用 token 数。
}

// MessageImage 表示用户消息携带的一张图片附件。
type MessageImage struct {
	ID        string    `json:"id"`        // ID 表示图片附件唯一标识。
	FileName  string    `json:"fileName"`  // FileName 表示图片文件名。
	MimeType  string    `json:"mimeType"`  // MimeType 表示图片 MIME 类型。
	Data      string    `json:"data"`      // Data 表示图片 base64 内容。
	CreatedAt time.Time `json:"createdAt"` // CreatedAt 表示创建时间。
	UpdatedAt time.Time `json:"updatedAt"` // UpdatedAt 表示更新时间。
}

// MessagePart 表示 assistant 消息中的一个顺序片段。
type MessagePart struct {
	ID        string    `json:"id"`                 // ID 表示片段唯一标识。
	Type      string    `json:"type"`               // Type 表示片段类型。
	Text      string    `json:"text,omitempty"`     // Text 表示文本片段内容。
	ToolCall  *ToolCall `json:"toolCall,omitempty"` // ToolCall 表示工具调用片段。
	CreatedAt time.Time `json:"createdAt"`          // CreatedAt 表示创建时间。
	UpdatedAt time.Time `json:"updatedAt"`          // UpdatedAt 表示更新时间。
}

// ChatMessage 表示聊天页中的一条消息。
type ChatMessage struct {
	ID        string         `json:"id"`                  // ID 表示消息唯一标识。
	ChatID    string         `json:"chatId"`              // ChatID 表示消息所属聊天页。
	Role      string         `json:"role"`                // Role 表示消息角色。
	Text      string         `json:"text"`                // Text 表示消息文本。
	Status    string         `json:"status"`              // Status 表示消息状态。
	ToolCalls []ToolCall     `json:"toolCalls,omitempty"` // ToolCalls 表示消息中的工具调用。
	Parts     []MessagePart  `json:"parts,omitempty"`     // Parts 表示 assistant 内容与工具调用的顺序片段。
	Images    []MessageImage `json:"images,omitempty"`    // Images 表示用户消息携带的图片附件。
	CreatedAt time.Time      `json:"createdAt"`           // CreatedAt 表示创建时间。
	UpdatedAt time.Time      `json:"updatedAt"`           // UpdatedAt 表示更新时间。
}

// PlanApproval 表示 plan 模式生成的待确认计划。
type PlanApproval struct {
	ID        string    `json:"id"`        // ID 表示待确认 plan 标识。
	MessageID string    `json:"messageId"` // MessageID 表示生成该 plan 的 assistant 消息标识。
	Text      string    `json:"text"`      // Text 表示 plan 正文。
	Status    string    `json:"status"`    // Status 表示 plan 当前状态。
	CreatedAt time.Time `json:"createdAt"` // CreatedAt 表示创建时间。
	UpdatedAt time.Time `json:"updatedAt"` // UpdatedAt 表示更新时间。
}

// Chat 表示 project 下的一个聊天页。
type Chat struct {
	ID             string             `json:"id"`                       // ID 表示聊天页唯一标识。
	ProjectID      string             `json:"projectId"`                // ProjectID 表示聊天页所属 project。
	Title          string             `json:"title"`                    // Title 表示聊天页标题。
	Status         string             `json:"status"`                   // Status 表示聊天页运行状态。
	AgentProvider  string             `json:"agentProvider"`            // AgentProvider 表示当前聊天页使用的 agent 类型。
	AgentModel     string             `json:"agentModel"`               // AgentModel 表示当前聊天页使用的模型。
	AgentReasoning string             `json:"agentReasoning,omitempty"` // AgentReasoning 表示当前聊天页使用的推理级别。
	AgentLocked    bool               `json:"agentLocked"`              // AgentLocked 表示会话开始后 agent 配置是否锁定。
	AgentSessionID string             `json:"agentSessionId,omitempty"` // AgentSessionID 表示 agent 会话标识。
	ContextWindow  ContextWindowUsage `json:"contextWindow"`            // ContextWindow 表示当前上下文窗口使用情况。
	Plan           *PlanApproval      `json:"plan,omitempty"`           // Plan 表示当前待确认或执行中的 plan。
	Messages       []ChatMessage      `json:"messages"`                 // Messages 表示聊天消息列表。
	CreatedAt      time.Time          `json:"createdAt"`                // CreatedAt 表示创建时间。
	UpdatedAt      time.Time          `json:"updatedAt"`                // UpdatedAt 表示更新时间。
}

// LastAgentSelection 表示新聊天页默认继承的 agent 配置。
type LastAgentSelection struct {
	Provider  string `json:"provider"`  // Provider 表示最近一次选择的 agent provider。
	Model     string `json:"model"`     // Model 表示最近一次选择的模型。
	Reasoning string `json:"reasoning"` // Reasoning 表示最近一次选择的推理级别。
}

// Snapshot 表示前端重连时需要恢复的完整内存状态。
type Snapshot struct {
	Projects           []Project             `json:"projects"`           // Projects 表示所有 project。
	Chats              []Chat                `json:"chats"`              // Chats 表示所有聊天页。
	AgentProviders     []AgentProviderOption `json:"agentProviders"`     // AgentProviders 表示可选 agent 和模型。
	AgentSkills        []AgentSkillOption    `json:"agentSkills"`        // AgentSkills 表示可在输入框中选择的 skills。
	LastAgentSelection LastAgentSelection    `json:"lastAgentSelection"` // LastAgentSelection 表示新聊天页默认 agent 配置。
}

// Store 维护 project、聊天页和消息的内存状态。
type Store struct {
	mu              sync.RWMutex
	projects        map[string]Project
	chats           map[string]Chat
	nextChatOrdinal map[string]int
	agentProviders  []AgentProviderOption
	lastAgent       LastAgentSelection
}

// NewStore 创建使用默认 agent 选项的内存状态存储。
func NewStore() *Store {
	return NewStoreWithAgentProviders(DefaultAgentProviderOptions())
}

// NewStoreWithAgentProviders 使用 agentProviders 参数创建内存状态存储。
func NewStoreWithAgentProviders(agentProviders []AgentProviderOption) *Store {
	if len(agentProviders) == 0 {
		agentProviders = DefaultAgentProviderOptions()
	}
	return &Store{
		projects:        make(map[string]Project),
		chats:           make(map[string]Chat),
		nextChatOrdinal: make(map[string]int),
		agentProviders:  cloneAgentProviderOptions(agentProviders),
		lastAgent:       defaultLastAgentSelection(agentProviders),
	}
}

// CreateProject 使用 projectPath 参数创建 project。
func (s *Store) CreateProject(projectPath string) (Project, error) {
	normalizedName, normalizedPath, err := validateProjectInput(projectPath)
	if err != nil {
		return Project{}, err
	}

	now := time.Now()
	project := Project{
		ID:        newID("project"),
		Name:      normalizedName,
		Path:      normalizedPath,
		Git:       resolveGitInfo(normalizedPath),
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[project.ID] = project
	return project, nil
}

// UpdateProject 使用 id 和 projectPath 参数更新 project。
func (s *Store) UpdateProject(id string, projectPath string) (Project, error) {
	normalizedName, normalizedPath, err := validateProjectInput(projectPath)
	if err != nil {
		return Project{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	project, ok := s.projects[id]
	if !ok {
		return Project{}, ErrNotFound
	}
	project.Name = normalizedName
	project.Path = normalizedPath
	project.Git = resolveGitInfo(normalizedPath)
	project.UpdatedAt = time.Now()
	s.projects[id] = project
	return project, nil
}

// DeleteProject 使用 id 参数删除 project 及其聊天页，并返回被删除的聊天页标识。
func (s *Store) DeleteProject(id string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[id]; !ok {
		return nil, ErrNotFound
	}
	delete(s.projects, id)
	delete(s.nextChatOrdinal, id)

	chatIDs := make([]string, 0)
	for chatID, chat := range s.chats {
		if chat.ProjectID == id {
			chatIDs = append(chatIDs, chatID)
			delete(s.chats, chatID)
		}
	}
	sort.Strings(chatIDs)
	return chatIDs, nil
}

// DeleteChat 使用 id 参数删除单个聊天页，并返回其所属 project 标识。
func (s *Store) DeleteChat(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[id]
	if !ok {
		return "", ErrNotFound
	}
	delete(s.chats, id)
	return chat.ProjectID, nil
}

// CreateChat 使用 projectID 参数创建聊天页。
func (s *Store) CreateChat(projectID string) (Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[projectID]; !ok {
		return Chat{}, ErrNotFound
	}
	s.nextChatOrdinal[projectID]++
	now := time.Now()
	lastAgent := s.lastAgent
	chat := Chat{
		ID:             newID("chat"),
		ProjectID:      projectID,
		Title:          fmt.Sprintf("聊天 %d", s.nextChatOrdinal[projectID]),
		Status:         ChatStatusIdle,
		AgentProvider:  lastAgent.Provider,
		AgentModel:     lastAgent.Model,
		AgentReasoning: lastAgent.Reasoning,
		ContextWindow:  ContextWindowUsage{MaxTokens: contextWindowMaxTokens(lastAgent.Provider, lastAgent.Model)},
		Messages:       []ChatMessage{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.chats[chat.ID] = chat
	return cloneChat(chat), nil
}

// AddAgentModel 使用 provider、modelID 和 label 参数新增 agent 模型选项。
func (s *Store) AddAgentModel(provider string, modelID string, label string) ([]AgentProviderOption, error) {
	normalizedProvider := strings.TrimSpace(provider)
	normalizedModelID := strings.TrimSpace(modelID)
	normalizedLabel := strings.TrimSpace(label)
	if normalizedProvider != AgentProviderClaudeCode {
		return nil, fmt.Errorf("%w: 只允许更新 Claude Code 模型列表", ErrInvalidInput)
	}
	if normalizedModelID == "" {
		return nil, fmt.Errorf("%w: 模型标识不能为空", ErrInvalidInput)
	}
	if normalizedLabel == "" {
		normalizedLabel = normalizedModelID
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for providerIndex := range s.agentProviders {
		option := &s.agentProviders[providerIndex]
		if option.ID != normalizedProvider {
			continue
		}
		for _, model := range option.Models {
			if model.ID == normalizedModelID {
				return nil, fmt.Errorf("%w: 模型已存在", ErrInvalidInput)
			}
		}
		option.Models = append(option.Models, AgentModelOption{
			ID:    normalizedModelID,
			Label: normalizedLabel,
		})
		return cloneAgentProviderOptions(s.agentProviders), nil
	}
	return nil, fmt.Errorf("%w: 不支持的 agent: %s", ErrInvalidInput, provider)
}

// UpdateChatAgent 使用 chatID、provider、model 和 reasoning 参数更新聊天页 agent 配置。
func (s *Store) UpdateChatAgent(chatID string, provider string, model string, reasoning string) (Chat, error) {
	normalizedProvider, normalizedModel, normalizedReasoning, err := NormalizeAgentSelection(provider, model, reasoning, s.agentProviders)
	if err != nil {
		return Chat{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, ErrNotFound
	}
	if chat.AgentLocked {
		if chat.AgentProvider != normalizedProvider {
			return Chat{}, fmt.Errorf("%w: agent 已锁定，不能切换 provider", ErrInvalidInput)
		}
	}
	chat.AgentProvider = normalizedProvider
	chat.AgentModel = normalizedModel
	chat.AgentReasoning = normalizedReasoning
	chat.ContextWindow = estimateChatContextWindowUsage(chat)
	chat.UpdatedAt = time.Now()
	s.chats[chatID] = chat
	s.lastAgent = LastAgentSelection{
		Provider:  normalizedProvider,
		Model:     normalizedModel,
		Reasoning: normalizedReasoning,
	}
	return cloneChat(chat), nil
}

// GetProjectAndChat 使用 chatID 参数读取聊天页及其 project。
func (s *Store) GetProjectAndChat(chatID string) (Project, Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Project{}, Chat{}, ErrNotFound
	}
	project, ok := s.projects[chat.ProjectID]
	if !ok {
		return Project{}, Chat{}, ErrNotFound
	}
	return project, cloneChat(chat), nil
}

// AddRunMessages 使用 chatID、prompt、images 和 planMode 参数追加用户消息和 assistant 占位消息。
func (s *Store) AddRunMessages(chatID string, prompt string, images []MessageImagePayload, planMode bool) (Chat, ChatMessage, ChatMessage, error) {
	trimmedPrompt := strings.TrimSpace(prompt)
	normalizedImages, err := normalizeMessageImages(images)
	if err != nil {
		return Chat{}, ChatMessage{}, ChatMessage{}, err
	}
	if trimmedPrompt == "" && len(normalizedImages) == 0 {
		return Chat{}, ChatMessage{}, ChatMessage{}, fmt.Errorf("%w: prompt 或图片不能为空", ErrInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, ChatMessage{}, ChatMessage{}, ErrNotFound
	}

	now := time.Now()
	for index := range normalizedImages {
		normalizedImages[index].CreatedAt = now
		normalizedImages[index].UpdatedAt = now
	}
	displayText := trimmedPrompt
	if displayText == "" {
		displayText = fmt.Sprintf("图片附件 %d 张", len(normalizedImages))
	}
	userMessage := ChatMessage{
		ID:        newID("msg"),
		ChatID:    chatID,
		Role:      MessageRoleUser,
		Text:      displayText,
		Status:    MessageStatusComplete,
		Images:    normalizedImages,
		CreatedAt: now,
		UpdatedAt: now,
	}
	assistantMessage := ChatMessage{
		ID:        newID("msg"),
		ChatID:    chatID,
		Role:      MessageRoleAssistant,
		Text:      "",
		Status:    MessageStatusStreaming,
		CreatedAt: now,
		UpdatedAt: now,
	}
	chat.Messages = append(chat.Messages, userMessage, assistantMessage)
	if len(chat.Messages) == 2 {
		if title := deriveChatTitleFromPrompt(displayText); title != "" {
			chat.Title = title
		}
	}
	chat.Status = ChatStatusRunning
	chat.AgentLocked = true
	if planMode {
		chat.Plan = nil
	}
	chat.ContextWindow = estimateChatContextWindowUsage(chat)
	chat.UpdatedAt = now
	s.chats[chatID] = chat
	return cloneChat(chat), userMessage, assistantMessage, nil
}

// AppendAssistantDelta 使用 chatID、messageID 和 delta 参数追加 assistant 流式文本。
func (s *Store) AppendAssistantDelta(chatID string, messageID string, delta string) (ChatMessage, bool) {
	if delta == "" {
		return ChatMessage{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return ChatMessage{}, false
	}
	for index := range chat.Messages {
		message := &chat.Messages[index]
		if message.ID != messageID || message.Role != MessageRoleAssistant {
			continue
		}
		if message.Status != MessageStatusStreaming {
			return ChatMessage{}, false
		}
		now := time.Now()
		message.Text += delta
		if len(message.Parts) > 0 && message.Parts[len(message.Parts)-1].Type == MessagePartTypeText {
			part := &message.Parts[len(message.Parts)-1]
			part.Text += delta
			part.UpdatedAt = now
		} else {
			message.Parts = append(message.Parts, MessagePart{
				ID:        newID("part"),
				Type:      MessagePartTypeText,
				Text:      delta,
				CreatedAt: now,
				UpdatedAt: now,
			})
		}
		message.UpdatedAt = now
		chat.ContextWindow = estimateChatContextWindowUsage(chat)
		chat.UpdatedAt = now
		s.chats[chatID] = chat
		return *message, true
	}
	return ChatMessage{}, false
}

// UpsertToolCall 使用 chatID、messageID 和 tool 参数插入或更新工具调用。
func (s *Store) UpsertToolCall(chatID string, messageID string, tool ToolCall) (Chat, ChatMessage, bool) {
	tool.Name = strings.TrimSpace(tool.Name)
	if strings.TrimSpace(tool.ID) == "" {
		tool.ID = newID("tool")
	}
	if strings.TrimSpace(tool.Status) == "" {
		tool.Status = ToolCallStatusRunning
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, ChatMessage{}, false
	}
	for messageIndex := range chat.Messages {
		message := &chat.Messages[messageIndex]
		if message.ID != messageID || message.Role != MessageRoleAssistant {
			continue
		}
		now := time.Now()
		updated := false
		var mergedTool ToolCall
		for toolIndex := range message.ToolCalls {
			if message.ToolCalls[toolIndex].ID != tool.ID {
				continue
			}
			existing := &message.ToolCalls[toolIndex]
			existing.Name = firstNonEmpty(tool.Name, existing.Name)
			existing.Status = firstNonEmpty(tool.Status, existing.Status)
			existing.Input = firstNonEmpty(tool.Input, existing.Input)
			existing.Output = firstNonEmpty(tool.Output, existing.Output)
			existing.UpdatedAt = now
			mergedTool = *existing
			updated = true
			break
		}
		if !updated {
			if tool.Name == "" {
				return Chat{}, ChatMessage{}, false
			}
			tool.CreatedAt = now
			tool.UpdatedAt = now
			message.ToolCalls = append(message.ToolCalls, tool)
			mergedTool = tool
		}
		upsertMessageToolPart(message, mergedTool, now)
		message.UpdatedAt = now
		chat.ContextWindow = estimateChatContextWindowUsage(chat)
		chat.UpdatedAt = now
		s.chats[chatID] = chat
		return cloneChat(chat), cloneChatMessage(*message), true
	}
	return Chat{}, ChatMessage{}, false
}

// FinishAssistantMessage 使用 chatID、messageID 和 status 参数结束 assistant 消息。
func (s *Store) FinishAssistantMessage(chatID string, messageID string, status string) (Chat, ChatMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, ChatMessage{}, false
	}
	for index := range chat.Messages {
		message := &chat.Messages[index]
		if message.ID != messageID || message.Role != MessageRoleAssistant {
			continue
		}
		if message.Status != MessageStatusStreaming {
			return cloneChat(chat), *message, false
		}
		now := time.Now()
		message.Status = status
		message.UpdatedAt = now
		if status == MessageStatusError {
			chat.Status = ChatStatusError
		} else {
			chat.Status = ChatStatusIdle
		}
		chat.UpdatedAt = now
		s.chats[chatID] = chat
		return cloneChat(chat), *message, true
	}
	return Chat{}, ChatMessage{}, false
}

// StopStreamingMessage 使用 chatID 和 status 参数停止聊天页中最后一条流式 assistant 消息。
func (s *Store) StopStreamingMessage(chatID string, status string) (Chat, ChatMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, ChatMessage{}, false
	}
	for index := len(chat.Messages) - 1; index >= 0; index-- {
		message := &chat.Messages[index]
		if message.Role != MessageRoleAssistant || message.Status != MessageStatusStreaming {
			continue
		}
		now := time.Now()
		message.Status = status
		message.UpdatedAt = now
		chat.Status = ChatStatusIdle
		chat.UpdatedAt = now
		s.chats[chatID] = chat
		return cloneChat(chat), *message, true
	}
	if chat.Status == ChatStatusRunning {
		chat.Status = ChatStatusIdle
		chat.UpdatedAt = time.Now()
		s.chats[chatID] = chat
		return cloneChat(chat), ChatMessage{}, true
	}
	return cloneChat(chat), ChatMessage{}, false
}

// AddSystemMessage 使用 chatID、text 和 status 参数追加系统消息。
func (s *Store) AddSystemMessage(chatID string, text string, status string) (Chat, ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, ChatMessage{}, ErrNotFound
	}
	now := time.Now()
	message := ChatMessage{
		ID:        newID("msg"),
		ChatID:    chatID,
		Role:      MessageRoleSystem,
		Text:      text,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
	chat.Messages = append(chat.Messages, message)
	chat.Status = ChatStatusError
	chat.UpdatedAt = now
	s.chats[chatID] = chat
	return cloneChat(chat), message, nil
}

// SetChatPlan 使用 chatID、messageID 和 text 参数记录当前待确认 plan。
func (s *Store) SetChatPlan(chatID string, messageID string, text string) (Chat, bool) {
	trimmedText := strings.TrimSpace(text)
	if trimmedText == "" {
		return Chat{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, false
	}
	now := time.Now()
	chat.Plan = &PlanApproval{
		ID:        newID("plan"),
		MessageID: messageID,
		Text:      trimmedText,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}
	chat.UpdatedAt = now
	s.chats[chatID] = chat
	return cloneChat(chat), true
}

// MarkPlanExecuting 使用 chatID 和 planID 参数把 plan 标记为执行中并返回 plan 正文。
func (s *Store) MarkPlanExecuting(chatID string, planID string) (Chat, PlanApproval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, PlanApproval{}, ErrNotFound
	}
	if chat.Plan == nil || chat.Plan.ID != planID {
		return Chat{}, PlanApproval{}, fmt.Errorf("%w: plan 不存在", ErrNotFound)
	}
	if chat.Plan.Status != "pending" {
		return Chat{}, PlanApproval{}, fmt.Errorf("%w: plan 当前不可执行", ErrInvalidInput)
	}
	now := time.Now()
	chat.Plan.Status = "executing"
	chat.Plan.UpdatedAt = now
	chat.UpdatedAt = now
	s.chats[chatID] = chat
	return cloneChat(chat), *chat.Plan, nil
}

// UpdateContextWindowUsage 使用 chatID 和 usage 参数更新 agent 上报的上下文窗口使用量。
func (s *Store) UpdateContextWindowUsage(chatID string, usage ContextWindowUsage) (Chat, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, false
	}
	if usage.MaxTokens <= 0 {
		usage.MaxTokens = chat.ContextWindow.MaxTokens
	}
	if usage.MaxTokens <= 0 {
		usage.MaxTokens = contextWindowMaxTokens(chat.AgentProvider, chat.AgentModel)
	}
	if usage.UsedTokens < 0 {
		usage.UsedTokens = 0
	}
	if usage.UsedTokens > usage.MaxTokens {
		usage.UsedTokens = usage.MaxTokens
	}
	if chat.ContextWindow == usage {
		return Chat{}, false
	}
	chat.ContextWindow = usage
	chat.UpdatedAt = time.Now()
	s.chats[chatID] = chat
	return cloneChat(chat), true
}

// SetChatSessionID 使用 chatID 和 sessionID 参数记录 Claude 会话标识。
func (s *Store) SetChatSessionID(chatID string, sessionID string) (Chat, bool) {
	if strings.TrimSpace(sessionID) == "" {
		return Chat{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok || chat.AgentSessionID == sessionID {
		return Chat{}, false
	}
	chat.AgentSessionID = sessionID
	chat.UpdatedAt = time.Now()
	s.chats[chatID] = chat
	return cloneChat(chat), true
}

// Snapshot 返回当前内存状态的稳定排序副本。
func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()

	projects := make([]Project, 0, len(s.projects))
	for _, project := range s.projects {
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i int, j int) bool {
		if projects[i].CreatedAt.Equal(projects[j].CreatedAt) {
			return projects[i].ID < projects[j].ID
		}
		return projects[i].CreatedAt.Before(projects[j].CreatedAt)
	})

	chats := make([]Chat, 0, len(s.chats))
	for _, chat := range s.chats {
		chats = append(chats, cloneChat(chat))
	}
	agentProviders := cloneAgentProviderOptions(s.agentProviders)
	lastAgent := s.lastAgent
	s.mu.RUnlock()

	sort.Slice(chats, func(i int, j int) bool {
		if chats[i].CreatedAt.Equal(chats[j].CreatedAt) {
			return chats[i].ID < chats[j].ID
		}
		return chats[i].CreatedAt.Before(chats[j].CreatedAt)
	})

	projectPaths := make([]string, 0, len(projects))
	for _, project := range projects {
		projectPaths = append(projectPaths, project.Path)
	}
	return Snapshot{
		Projects:           projects,
		Chats:              chats,
		AgentProviders:     agentProviders,
		AgentSkills:        LoadAgentSkillOptions(projectPaths),
		LastAgentSelection: lastAgent,
	}
}

// AgentSkills 返回当前 project 和 CODEX_HOME 中可用的 skill 列表。
func (s *Store) AgentSkills() []AgentSkillOption {
	s.mu.RLock()
	projectPaths := make([]string, 0, len(s.projects))
	for _, project := range s.projects {
		projectPaths = append(projectPaths, project.Path)
	}
	s.mu.RUnlock()
	return LoadAgentSkillOptions(projectPaths)
}

// validateProjectInput 使用 projectPath 参数校验 project 输入，并返回派生名称和绝对路径。
func validateProjectInput(projectPath string) (string, string, error) {
	normalizedPath := strings.TrimSpace(projectPath)
	if normalizedPath == "" {
		return "", "", fmt.Errorf("%w: project 路径不能为空", ErrInvalidInput)
	}
	absPath, err := filepath.Abs(normalizedPath)
	if err != nil {
		return "", "", fmt.Errorf("%w: 解析 project 路径失败", ErrInvalidInput)
	}
	stat, err := os.Stat(absPath)
	if err != nil {
		return "", "", fmt.Errorf("%w: project 路径不存在", ErrInvalidInput)
	}
	if !stat.IsDir() {
		return "", "", fmt.Errorf("%w: project 路径不是目录", ErrInvalidInput)
	}
	return filepath.Base(absPath), absPath, nil
}

// cloneChat 使用 chat 参数创建不会共享消息切片的副本。
func cloneChat(chat Chat) Chat {
	if chat.Plan != nil {
		plan := *chat.Plan
		chat.Plan = &plan
	}
	messages := chat.Messages
	chat.Messages = make([]ChatMessage, 0, len(messages))
	for _, message := range messages {
		chat.Messages = append(chat.Messages, cloneChatMessage(message))
	}
	if chat.Messages == nil {
		chat.Messages = []ChatMessage{}
	}
	return chat
}

// cloneChatMessage 使用 message 参数创建不会共享工具调用切片的副本。
func cloneChatMessage(message ChatMessage) ChatMessage {
	message.ToolCalls = append([]ToolCall(nil), message.ToolCalls...)
	message.Parts = cloneMessageParts(message.Parts)
	message.Images = append([]MessageImage(nil), message.Images...)
	return message
}

// cloneMessageParts 使用 parts 参数创建不会共享工具调用指针的片段副本。
func cloneMessageParts(parts []MessagePart) []MessagePart {
	if len(parts) == 0 {
		return nil
	}
	result := make([]MessagePart, 0, len(parts))
	for _, part := range parts {
		if part.ToolCall != nil {
			tool := *part.ToolCall
			part.ToolCall = &tool
		}
		result = append(result, part)
	}
	return result
}

// normalizeMessageImages 使用 images 参数校验并标准化图片附件。
func normalizeMessageImages(images []MessageImagePayload) ([]MessageImage, error) {
	if len(images) == 0 {
		return nil, nil
	}
	if len(images) > 8 {
		return nil, fmt.Errorf("%w: 图片最多支持 8 张", ErrInvalidInput)
	}
	result := make([]MessageImage, 0, len(images))
	for index, image := range images {
		mimeType := strings.TrimSpace(image.MimeType)
		if !strings.HasPrefix(mimeType, "image/") {
			return nil, fmt.Errorf("%w: 只支持图片附件", ErrInvalidInput)
		}
		data := strings.TrimSpace(image.Data)
		if data == "" {
			return nil, fmt.Errorf("%w: 图片内容不能为空", ErrInvalidInput)
		}
		if len(data) > 8*1024*1024 {
			return nil, fmt.Errorf("%w: 单张图片过大", ErrInvalidInput)
		}
		fileName := strings.TrimSpace(image.FileName)
		if fileName == "" {
			fileName = fmt.Sprintf("image-%d", index+1)
		}
		id := strings.TrimSpace(image.ID)
		if id == "" {
			id = newID("img")
		}
		result = append(result, MessageImage{
			ID:       id,
			FileName: fileName,
			MimeType: mimeType,
			Data:     data,
		})
	}
	return result, nil
}

// estimateChatContextWindowUsage 使用 chat 参数估算上下文窗口使用量。
func estimateChatContextWindowUsage(chat Chat) ContextWindowUsage {
	maxTokens := contextWindowMaxTokens(chat.AgentProvider, chat.AgentModel)
	usedTokens := 0
	for _, message := range chat.Messages {
		usedTokens += estimateTextTokens(message.Text)
		usedTokens += len(message.Images) * 85
		for _, tool := range message.ToolCalls {
			usedTokens += estimateTextTokens(tool.Input)
			usedTokens += estimateTextTokens(tool.Output)
		}
	}
	if usedTokens > maxTokens {
		usedTokens = maxTokens
	}
	return ContextWindowUsage{MaxTokens: maxTokens, UsedTokens: usedTokens}
}

// estimateTextTokens 使用 text 参数粗略估算 token 数。
func estimateTextTokens(text string) int {
	runeCount := len([]rune(strings.TrimSpace(text)))
	if runeCount == 0 {
		return 0
	}
	return runeCount/4 + 1
}

// contextWindowMaxTokens 使用 provider 和 model 参数返回模型上下文窗口上限。
func contextWindowMaxTokens(provider string, model string) int {
	normalized := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(normalized, "opus"):
		return 1_000_000
	case strings.Contains(normalized, "gpt-5.5"), strings.Contains(normalized, "mock-codex"):
		return 258_000
	case strings.Contains(provider, "codex"):
		return 200_000
	default:
		return 200_000
	}
}

// upsertMessageToolPart 使用 message、tool 和 now 参数更新消息中的工具调用片段。
func upsertMessageToolPart(message *ChatMessage, tool ToolCall, now time.Time) {
	for index := range message.Parts {
		part := &message.Parts[index]
		if part.Type != MessagePartTypeToolCall || part.ToolCall == nil || part.ToolCall.ID != tool.ID {
			continue
		}
		part.ToolCall = &tool
		part.UpdatedAt = now
		return
	}
	message.Parts = append(message.Parts, MessagePart{
		ID:        newID("part"),
		Type:      MessagePartTypeToolCall,
		ToolCall:  &tool,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// cloneAgentProviderOptions 使用 options 参数创建不会共享模型和推理级别切片的副本。
func cloneAgentProviderOptions(options []AgentProviderOption) []AgentProviderOption {
	result := make([]AgentProviderOption, 0, len(options))
	for _, option := range options {
		option.Models = append([]AgentModelOption(nil), option.Models...)
		for index := range option.Models {
			option.Models[index].ReasoningLevels = append([]AgentReasoningOption(nil), option.Models[index].ReasoningLevels...)
		}
		result = append(result, option)
	}
	return result
}

// newID 使用 prefix 参数生成短随机标识。
func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}

// resolveGitInfo 使用 projectPath 参数读取 Git 仓库摘要。
func resolveGitInfo(projectPath string) GitInfo {
	if _, err := gitOutput(projectPath, "rev-parse", "--show-toplevel"); err != nil {
		return GitInfo{}
	}
	branch, _ := gitOutput(projectPath, "rev-parse", "--abbrev-ref", "HEAD")
	commit, _ := gitOutput(projectPath, "rev-parse", "--short", "HEAD")
	status, _ := gitOutput(projectPath, "status", "--short")
	return GitInfo{
		IsRepo: true,
		Branch: strings.TrimSpace(branch),
		Commit: strings.TrimSpace(commit),
		Dirty:  strings.TrimSpace(status) != "",
	}
}

// gitOutput 使用 projectPath 和 args 参数运行 Git 命令并返回裁剪后的输出。
func gitOutput(projectPath string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", projectPath}, args...)
	cmd := exec.Command("git", fullArgs...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
