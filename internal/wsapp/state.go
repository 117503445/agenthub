package wsapp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
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
	// MessageStatusComplete 表示消息已经完成。
	MessageStatusComplete = "complete"
	// MessageStatusStreaming 表示消息正在流式输出。
	MessageStatusStreaming = "streaming"
	// MessageStatusStopped 表示消息被用户停止。
	MessageStatusStopped = "stopped"
	// MessageStatusError 表示消息输出失败。
	MessageStatusError = "error"
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
	Name      string    `json:"name"`      // Name 表示 project 展示名称。
	Path      string    `json:"path"`      // Path 表示 project 工作目录。
	CreatedAt time.Time `json:"createdAt"` // CreatedAt 表示创建时间。
	UpdatedAt time.Time `json:"updatedAt"` // UpdatedAt 表示更新时间。
}

// ChatMessage 表示聊天页中的一条消息。
type ChatMessage struct {
	ID        string    `json:"id"`        // ID 表示消息唯一标识。
	ChatID    string    `json:"chatId"`    // ChatID 表示消息所属聊天页。
	Role      string    `json:"role"`      // Role 表示消息角色。
	Text      string    `json:"text"`      // Text 表示消息文本。
	Status    string    `json:"status"`    // Status 表示消息状态。
	CreatedAt time.Time `json:"createdAt"` // CreatedAt 表示创建时间。
	UpdatedAt time.Time `json:"updatedAt"` // UpdatedAt 表示更新时间。
}

// Chat 表示 project 下的一个聊天页。
type Chat struct {
	ID             string        `json:"id"`                       // ID 表示聊天页唯一标识。
	ProjectID      string        `json:"projectId"`                // ProjectID 表示聊天页所属 project。
	Title          string        `json:"title"`                    // Title 表示聊天页标题。
	Status         string        `json:"status"`                   // Status 表示聊天页运行状态。
	AgentSessionID string        `json:"agentSessionId,omitempty"` // AgentSessionID 表示 Claude 会话标识。
	Messages       []ChatMessage `json:"messages"`                 // Messages 表示聊天消息列表。
	CreatedAt      time.Time     `json:"createdAt"`                // CreatedAt 表示创建时间。
	UpdatedAt      time.Time     `json:"updatedAt"`                // UpdatedAt 表示更新时间。
}

// Snapshot 表示前端重连时需要恢复的完整内存状态。
type Snapshot struct {
	Projects []Project `json:"projects"` // Projects 表示所有 project。
	Chats    []Chat    `json:"chats"`    // Chats 表示所有聊天页。
}

// Store 维护 project、聊天页和消息的内存状态。
type Store struct {
	mu              sync.RWMutex
	projects        map[string]Project
	chats           map[string]Chat
	nextChatOrdinal map[string]int
}

// NewStore 创建空的内存状态存储。
func NewStore() *Store {
	return &Store{
		projects:        make(map[string]Project),
		chats:           make(map[string]Chat),
		nextChatOrdinal: make(map[string]int),
	}
}

// CreateProject 使用 name 和 projectPath 参数创建 project。
func (s *Store) CreateProject(name string, projectPath string) (Project, error) {
	normalizedName, normalizedPath, err := validateProjectInput(name, projectPath)
	if err != nil {
		return Project{}, err
	}

	now := time.Now()
	project := Project{
		ID:        newID("project"),
		Name:      normalizedName,
		Path:      normalizedPath,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[project.ID] = project
	return project, nil
}

// UpdateProject 使用 id、name 和 projectPath 参数更新 project。
func (s *Store) UpdateProject(id string, name string, projectPath string) (Project, error) {
	normalizedName, normalizedPath, err := validateProjectInput(name, projectPath)
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
		ID:        newID("chat"),
		ProjectID: projectID,
		Title:     fmt.Sprintf("聊天 %d", s.nextChatOrdinal[projectID]),
		Status:    ChatStatusIdle,
		Messages:  []ChatMessage{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.chats[chat.ID] = chat
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
	chat.Status = ChatStatusRunning
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
		message.UpdatedAt = now
		chat.UpdatedAt = now
		s.chats[chatID] = chat
		return *message, true
	}
	return ChatMessage{}, false
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

	return Snapshot{Projects: projects, Chats: chats}
}

// validateProjectInput 使用 name 和 projectPath 参数校验 project 输入。
func validateProjectInput(name string, projectPath string) (string, string, error) {
	normalizedName := strings.TrimSpace(name)
	normalizedPath := strings.TrimSpace(projectPath)
	if normalizedName == "" {
		return "", "", fmt.Errorf("%w: project 名称不能为空", ErrInvalidInput)
	}
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
	return normalizedName, absPath, nil
}

// cloneChat 使用 chat 参数创建不会共享消息切片的副本。
func cloneChat(chat Chat) Chat {
	chat.Messages = append([]ChatMessage(nil), chat.Messages...)
	if chat.Messages == nil {
		chat.Messages = []ChatMessage{}
	}
	return chat
}

// newID 使用 prefix 参数生成短随机标识。
func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
