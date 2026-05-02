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
	ID        string        `json:"id"`                  // ID 表示消息唯一标识。
	ChatID    string        `json:"chatId"`              // ChatID 表示消息所属聊天页。
	Role      string        `json:"role"`                // Role 表示消息角色。
	Text      string        `json:"text"`                // Text 表示消息文本。
	Status    string        `json:"status"`              // Status 表示消息状态。
	ToolCalls []ToolCall    `json:"toolCalls,omitempty"` // ToolCalls 表示消息中的工具调用。
	Parts     []MessagePart `json:"parts,omitempty"`     // Parts 表示 assistant 内容与工具调用的顺序片段。
	CreatedAt time.Time     `json:"createdAt"`           // CreatedAt 表示创建时间。
	UpdatedAt time.Time     `json:"updatedAt"`           // UpdatedAt 表示更新时间。
}

// Chat 表示 project 下的一个聊天页。
type Chat struct {
	ID             string        `json:"id"`                       // ID 表示聊天页唯一标识。
	ProjectID      string        `json:"projectId"`                // ProjectID 表示聊天页所属 project。
	Title          string        `json:"title"`                    // Title 表示聊天页标题。
	Status         string        `json:"status"`                   // Status 表示聊天页运行状态。
	AgentProvider  string        `json:"agentProvider"`            // AgentProvider 表示当前聊天页使用的 agent 类型。
	AgentModel     string        `json:"agentModel"`               // AgentModel 表示当前聊天页使用的模型。
	AgentReasoning string        `json:"agentReasoning,omitempty"` // AgentReasoning 表示当前聊天页使用的推理级别。
	AgentLocked    bool          `json:"agentLocked"`              // AgentLocked 表示会话开始后 agent 配置是否锁定。
	AgentSessionID string        `json:"agentSessionId,omitempty"` // AgentSessionID 表示 agent 会话标识。
	Messages       []ChatMessage `json:"messages"`                 // Messages 表示聊天消息列表。
	CreatedAt      time.Time     `json:"createdAt"`                // CreatedAt 表示创建时间。
	UpdatedAt      time.Time     `json:"updatedAt"`                // UpdatedAt 表示更新时间。
}

// Snapshot 表示前端重连时需要恢复的完整内存状态。
type Snapshot struct {
	Projects       []Project             `json:"projects"`       // Projects 表示所有 project。
	Chats          []Chat                `json:"chats"`          // Chats 表示所有聊天页。
	AgentProviders []AgentProviderOption `json:"agentProviders"` // AgentProviders 表示可选 agent 和模型。
}

// Store 维护 project、聊天页和消息的内存状态。
type Store struct {
	mu              sync.RWMutex
	projects        map[string]Project
	chats           map[string]Chat
	nextChatOrdinal map[string]int
	agentProviders  []AgentProviderOption
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

// CreateChat 使用 projectID 参数创建聊天页。
func (s *Store) CreateChat(projectID string) (Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[projectID]; !ok {
		return Chat{}, ErrNotFound
	}
	s.nextChatOrdinal[projectID]++
	now := time.Now()
	chat := Chat{
		ID:            newID("chat"),
		ProjectID:     projectID,
		Title:         fmt.Sprintf("聊天 %d", s.nextChatOrdinal[projectID]),
		Status:        ChatStatusIdle,
		AgentProvider: AgentProviderMockClaudeCode,
		AgentModel:    DefaultAgentModel(AgentProviderMockClaudeCode, s.agentProviders),
		Messages:      []ChatMessage{},
		CreatedAt:     now,
		UpdatedAt:     now,
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
	chat.UpdatedAt = time.Now()
	s.chats[chatID] = chat
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

// AddRunMessages 使用 chatID 和 prompt 参数追加用户消息和 assistant 占位消息。
func (s *Store) AddRunMessages(chatID string, prompt string) (Chat, ChatMessage, ChatMessage, error) {
	trimmedPrompt := strings.TrimSpace(prompt)
	if trimmedPrompt == "" {
		return Chat{}, ChatMessage{}, ChatMessage{}, fmt.Errorf("%w: prompt 不能为空", ErrInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return Chat{}, ChatMessage{}, ChatMessage{}, ErrNotFound
	}

	now := time.Now()
	userMessage := ChatMessage{
		ID:        newID("msg"),
		ChatID:    chatID,
		Role:      MessageRoleUser,
		Text:      trimmedPrompt,
		Status:    MessageStatusComplete,
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
		if title := deriveChatTitleFromPrompt(trimmedPrompt); title != "" {
			chat.Title = title
		}
	}
	chat.Status = ChatStatusRunning
	chat.AgentLocked = true
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
	defer s.mu.RUnlock()

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
	sort.Slice(chats, func(i int, j int) bool {
		if chats[i].CreatedAt.Equal(chats[j].CreatedAt) {
			return chats[i].ID < chats[j].ID
		}
		return chats[i].CreatedAt.Before(chats[j].CreatedAt)
	})

	return Snapshot{Projects: projects, Chats: chats, AgentProviders: cloneAgentProviderOptions(s.agentProviders)}
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
