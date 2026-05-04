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
	// errStoreUnchanged 表示本次提交没有状态变化。
	errStoreUnchanged = errors.New("状态未变化")
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
	AgentProvider  string             `json:"agentProvider"`            // AgentProvider 表示当前聊天页使用的 Profile 标识。
	AgentModel     string             `json:"agentModel"`               // AgentModel 表示当前聊天页使用的模型。
	AgentReasoning string             `json:"agentReasoning,omitempty"` // AgentReasoning 表示当前聊天页使用的推理级别。
	AgentLocked    bool               `json:"agentLocked"`              // AgentLocked 表示会话开始后 agent 配置是否锁定。
	AgentSessionID string             `json:"agentSessionId,omitempty"` // AgentSessionID 表示 agent 会话标识。
	AgentProfile   AgentProfile       `json:"agentProfile,omitempty"`   // AgentProfile 表示聊天页绑定的 Profile 快照。
	ContextWindow  ContextWindowUsage `json:"contextWindow"`            // ContextWindow 表示当前上下文窗口使用情况。
	Plan           *PlanApproval      `json:"plan,omitempty"`           // Plan 表示当前待确认或执行中的 plan。
	Messages       []ChatMessage      `json:"messages"`                 // Messages 表示聊天消息列表。
	CreatedAt      time.Time          `json:"createdAt"`                // CreatedAt 表示创建时间。
	UpdatedAt      time.Time          `json:"updatedAt"`                // UpdatedAt 表示更新时间。
}

// LastAgentSelection 表示新聊天页默认继承的 agent 配置。
type LastAgentSelection struct {
	Provider  string `json:"provider"`  // Provider 表示最近一次选择的 Profile 标识。
	Model     string `json:"model"`     // Model 表示最近一次选择的模型。
	Reasoning string `json:"reasoning"` // Reasoning 表示最近一次选择的推理级别。
}

// Snapshot 表示前端重连时需要恢复的完整内存状态。
type Snapshot struct {
	Projects           []Project             `json:"projects"`           // Projects 表示所有 project。
	Chats              []Chat                `json:"chats"`              // Chats 表示所有聊天页。
	AgentProviders     []AgentProviderOption `json:"agentProviders"`     // AgentProviders 表示可选 agent 和模型。
	AgentProfiles      []AgentProfile        `json:"agentProfiles"`      // AgentProfiles 表示可编辑的 Profile 列表。
	BackendEnv         []BackendEnvVar       `json:"backendEnv"`         // BackendEnv 表示后端启动时的环境变量。
	AgentSkills        []AgentSkillOption    `json:"agentSkills"`        // AgentSkills 表示可在输入框中选择的 skills。
	LastAgentSelection LastAgentSelection    `json:"lastAgentSelection"` // LastAgentSelection 表示新聊天页默认 agent 配置。
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

	if err := s.commit(func(state *storeState) error {
		state.projects[project.ID] = project
		return nil
	}); err != nil {
		return Project{}, err
	}
	return project, nil
}

// CreateProjectFromGitWorkdir 使用 workdir 参数在 Git 仓库目录中创建 project 和首个聊天页。
func (s *Store) CreateProjectFromGitWorkdir(workdir string) (Project, Chat, bool, error) {
	normalizedName, normalizedPath, err := validateProjectInput(workdir)
	if err != nil {
		return Project{}, Chat{}, false, err
	}
	gitInfo := resolveGitInfo(normalizedPath)
	if !gitInfo.IsRepo {
		return Project{}, Chat{}, false, nil
	}

	now := time.Now()
	project := Project{
		ID:        newID("project"),
		Name:      normalizedName,
		Path:      normalizedPath,
		Git:       gitInfo,
		CreatedAt: now,
		UpdatedAt: now,
	}

	var chat Chat
	created := true
	err = s.commit(func(state *storeState) error {
		for _, existing := range state.projects {
			if existing.Path == normalizedPath {
				project = existing
				created = false
				return errStoreUnchanged
			}
		}
		state.projects[project.ID] = project
		chat = createChatInState(state, project.ID, now)
		return nil
	})
	if err != nil {
		if errors.Is(err, errStoreUnchanged) && !created {
			return project, Chat{}, false, nil
		}
		return Project{}, Chat{}, false, err
	}
	if !created {
		return project, Chat{}, false, nil
	}
	return project, cloneChat(chat), true, nil
}

// UpdateProject 使用 id 和 projectPath 参数更新 project。
func (s *Store) UpdateProject(id string, projectPath string) (Project, error) {
	normalizedName, normalizedPath, err := validateProjectInput(projectPath)
	if err != nil {
		return Project{}, err
	}

	var project Project
	if err := s.commit(func(state *storeState) error {
		var ok bool
		project, ok = state.projects[id]
		if !ok {
			return ErrNotFound
		}
		project.Name = normalizedName
		project.Path = normalizedPath
		project.Git = resolveGitInfo(normalizedPath)
		project.UpdatedAt = time.Now()
		state.projects[id] = project
		return nil
	}); err != nil {
		return Project{}, err
	}
	return project, nil
}

// DeleteProject 使用 id 参数删除 project 及其聊天页，并返回被删除的聊天页标识。
func (s *Store) DeleteProject(id string) ([]string, error) {
	chatIDs := make([]string, 0)
	if err := s.commit(func(state *storeState) error {
		if _, ok := state.projects[id]; !ok {
			return ErrNotFound
		}
		delete(state.projects, id)
		delete(state.nextChatOrdinal, id)

		for chatID, chat := range state.chats {
			if chat.ProjectID == id {
				chatIDs = append(chatIDs, chatID)
				delete(state.chats, chatID)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(chatIDs)
	return chatIDs, nil
}

// DeleteChat 使用 id 参数删除单个聊天页，并返回其所属 project 标识。
func (s *Store) DeleteChat(id string) (string, error) {
	var projectID string
	if err := s.commit(func(state *storeState) error {
		chat, ok := state.chats[id]
		if !ok {
			return ErrNotFound
		}
		delete(state.chats, id)
		projectID = chat.ProjectID
		return nil
	}); err != nil {
		return "", err
	}
	return projectID, nil
}

// CreateChat 使用 projectID 参数创建聊天页。
func (s *Store) CreateChat(projectID string) (Chat, error) {
	var chat Chat
	if err := s.commit(func(state *storeState) error {
		if _, ok := state.projects[projectID]; !ok {
			return ErrNotFound
		}
		chat = createChatInState(state, projectID, time.Now())
		return nil
	}); err != nil {
		return Chat{}, err
	}
	return cloneChat(chat), nil
}

// createChatInState 使用 state、projectID 和 now 参数创建聊天页。
func createChatInState(state *storeState, projectID string, now time.Time) Chat {
	state.nextChatOrdinal[projectID]++
	lastAgent := state.lastAgent
	chat := Chat{
		ID:             newID("chat"),
		ProjectID:      projectID,
		Title:          fmt.Sprintf("聊天 %d", state.nextChatOrdinal[projectID]),
		Status:         ChatStatusIdle,
		AgentProvider:  lastAgent.Provider,
		AgentModel:     lastAgent.Model,
		AgentReasoning: lastAgent.Reasoning,
		ContextWindow:  ContextWindowUsage{MaxTokens: contextWindowMaxTokens(lastAgent.Provider, lastAgent.Model)},
		Messages:       []ChatMessage{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	state.chats[chat.ID] = chat
	return chat
}

// AddAgentModel 使用 provider、modelID 和 label 参数新增 agent 模型选项。
func (s *Store) AddAgentModel(provider string, modelID string, label string) ([]AgentProviderOption, error) {
	normalizedProvider := strings.TrimSpace(provider)
	normalizedModelID := strings.TrimSpace(modelID)
	normalizedLabel := strings.TrimSpace(label)
	if normalizedModelID == "" {
		return nil, fmt.Errorf("%w: 模型标识不能为空", ErrInvalidInput)
	}
	if normalizedLabel == "" {
		normalizedLabel = normalizedModelID
	}

	var options []AgentProviderOption
	if err := s.commit(func(state *storeState) error {
		for profileIndex := range state.agentProfiles {
			profile := &state.agentProfiles[profileIndex]
			if profile.ID != normalizedProvider {
				continue
			}
			for _, model := range profile.Models {
				if model.ID == normalizedModelID {
					return fmt.Errorf("%w: 模型已存在", ErrInvalidInput)
				}
			}
			profile.Models = append(profile.Models, AgentModelOption{
				ID:    normalizedModelID,
				Label: normalizedLabel,
			})
			normalizedProfile, err := normalizeAgentProfile(*profile)
			if err != nil {
				return err
			}
			state.agentProfiles[profileIndex] = normalizedProfile
			applyProfileToChats(state, normalizedProfile)
			options = AgentProviderOptionsFromProfiles(state.agentProfiles)
			return nil
		}
		return fmt.Errorf("%w: 不支持的 agent: %s", ErrInvalidInput, provider)
	}); err != nil {
		return nil, err
	}
	return options, nil
}

// CreateAgentProfile 使用 profile 参数新增 Profile。
func (s *Store) CreateAgentProfile(profile AgentProfile) ([]AgentProfile, error) {
	normalized, err := normalizeAgentProfile(profile)
	if err != nil {
		return nil, err
	}
	var profiles []AgentProfile
	if err := s.commit(func(state *storeState) error {
		if _, ok := AgentProfileByID(state.agentProfiles, normalized.ID); ok {
			return fmt.Errorf("%w: Profile 已存在: %s", ErrInvalidInput, normalized.ID)
		}
		state.agentProfiles = append(state.agentProfiles, normalized)
		profiles = cloneAgentProfiles(state.agentProfiles)
		return nil
	}); err != nil {
		return nil, err
	}
	return profiles, nil
}

// UpdateAgentProfile 使用 profile 参数更新 Profile。
func (s *Store) UpdateAgentProfile(profile AgentProfile) ([]AgentProfile, error) {
	normalized, err := normalizeAgentProfile(profile)
	if err != nil {
		return nil, err
	}
	var profiles []AgentProfile
	if err := s.commit(func(state *storeState) error {
		for index := range state.agentProfiles {
			if state.agentProfiles[index].ID != normalized.ID {
				continue
			}
			state.agentProfiles[index] = normalized
			applyProfileToChats(state, normalized)
			profiles = cloneAgentProfiles(state.agentProfiles)
			return nil
		}
		return ErrNotFound
	}); err != nil {
		return nil, err
	}
	return profiles, nil
}

// DeleteAgentProfile 使用 profileID 参数删除 Profile。
func (s *Store) DeleteAgentProfile(profileID string) ([]AgentProfile, error) {
	normalizedID := strings.TrimSpace(profileID)
	if normalizedID == "" {
		return nil, fmt.Errorf("%w: Profile ID 不能为空", ErrInvalidInput)
	}
	var profiles []AgentProfile
	if err := s.commit(func(state *storeState) error {
		index := -1
		for itemIndex, profile := range state.agentProfiles {
			if profile.ID == normalizedID {
				index = itemIndex
				break
			}
		}
		if index < 0 {
			return ErrNotFound
		}
		state.agentProfiles = append(state.agentProfiles[:index], state.agentProfiles[index+1:]...)
		for chatID, chat := range state.chats {
			if chat.AgentLocked || chat.AgentProvider != normalizedID {
				continue
			}
			defaultAgent := defaultLastAgentSelection(AgentProviderOptionsFromProfiles(state.agentProfiles))
			chat.AgentProvider = defaultAgent.Provider
			chat.AgentModel = defaultAgent.Model
			chat.AgentReasoning = defaultAgent.Reasoning
			chat.AgentProfile = AgentProfile{}
			chat.ContextWindow = estimateChatContextWindowUsage(chat)
			chat.UpdatedAt = time.Now()
			state.chats[chatID] = chat
		}
		profiles = cloneAgentProfiles(state.agentProfiles)
		return nil
	}); err != nil {
		return nil, err
	}
	return profiles, nil
}

// AddAgentProfileModel 使用 profileID、modelID 和 label 参数新增模型。
func (s *Store) AddAgentProfileModel(profileID string, modelID string, label string) ([]AgentProfile, error) {
	normalizedModelID := strings.TrimSpace(modelID)
	normalizedLabel := strings.TrimSpace(label)
	if normalizedModelID == "" {
		return nil, fmt.Errorf("%w: 模型标识不能为空", ErrInvalidInput)
	}
	if normalizedLabel == "" {
		normalizedLabel = normalizedModelID
	}
	var profiles []AgentProfile
	if err := s.commit(func(state *storeState) error {
		profile, index, ok := profileByIDInState(state, profileID)
		if !ok {
			return ErrNotFound
		}
		for _, model := range profile.Models {
			if model.ID == normalizedModelID {
				return fmt.Errorf("%w: 模型已存在", ErrInvalidInput)
			}
		}
		profile.Models = append(profile.Models, AgentModelOption{ID: normalizedModelID, Label: normalizedLabel})
		normalized, err := normalizeAgentProfile(profile)
		if err != nil {
			return err
		}
		state.agentProfiles[index] = normalized
		applyProfileToChats(state, normalized)
		profiles = cloneAgentProfiles(state.agentProfiles)
		return nil
	}); err != nil {
		return nil, err
	}
	return profiles, nil
}

// UpdateAgentProfileModel 使用 profileID、modelID、label 和 defaultModel 参数更新模型。
func (s *Store) UpdateAgentProfileModel(profileID string, modelID string, label string, defaultModel bool) ([]AgentProfile, error) {
	normalizedModelID := strings.TrimSpace(modelID)
	normalizedLabel := strings.TrimSpace(label)
	if normalizedModelID == "" {
		return nil, fmt.Errorf("%w: 模型标识不能为空", ErrInvalidInput)
	}
	var profiles []AgentProfile
	if err := s.commit(func(state *storeState) error {
		profile, index, ok := profileByIDInState(state, profileID)
		if !ok {
			return ErrNotFound
		}
		modelIndex := -1
		for itemIndex := range profile.Models {
			if profile.Models[itemIndex].ID == normalizedModelID {
				modelIndex = itemIndex
				break
			}
		}
		if modelIndex < 0 {
			return ErrNotFound
		}
		if normalizedLabel != "" {
			profile.Models[modelIndex].Label = normalizedLabel
		}
		if defaultModel {
			for itemIndex := range profile.Models {
				profile.Models[itemIndex].Default = itemIndex == modelIndex
			}
		}
		normalized, err := normalizeAgentProfile(profile)
		if err != nil {
			return err
		}
		state.agentProfiles[index] = normalized
		applyProfileToChats(state, normalized)
		profiles = cloneAgentProfiles(state.agentProfiles)
		return nil
	}); err != nil {
		return nil, err
	}
	return profiles, nil
}

// DeleteAgentProfileModel 使用 profileID 和 modelID 参数删除模型。
func (s *Store) DeleteAgentProfileModel(profileID string, modelID string) ([]AgentProfile, error) {
	normalizedModelID := strings.TrimSpace(modelID)
	if normalizedModelID == "" {
		return nil, fmt.Errorf("%w: 模型标识不能为空", ErrInvalidInput)
	}
	var profiles []AgentProfile
	if err := s.commit(func(state *storeState) error {
		profile, index, ok := profileByIDInState(state, profileID)
		if !ok {
			return ErrNotFound
		}
		modelIndex := -1
		for itemIndex, model := range profile.Models {
			if model.ID == normalizedModelID {
				modelIndex = itemIndex
				break
			}
		}
		if modelIndex < 0 {
			return ErrNotFound
		}
		if len(profile.Models) == 1 {
			return fmt.Errorf("%w: Profile 至少需要一个模型", ErrInvalidInput)
		}
		profile.Models = append(profile.Models[:modelIndex], profile.Models[modelIndex+1:]...)
		if !hasDefaultAgentModel(profile.Models) {
			profile.Models[0].Default = true
		}
		normalized, err := normalizeAgentProfile(profile)
		if err != nil {
			return err
		}
		state.agentProfiles[index] = normalized
		applyProfileToChats(state, normalized)
		profiles = cloneAgentProfiles(state.agentProfiles)
		return nil
	}); err != nil {
		return nil, err
	}
	return profiles, nil
}

// UpdateChatAgent 使用 chatID、provider、model 和 reasoning 参数更新聊天页 agent 配置。
func (s *Store) UpdateChatAgent(chatID string, provider string, model string, reasoning string) (Chat, error) {
	var chat Chat
	if err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok {
			return ErrNotFound
		}
		profiles := state.agentProfiles
		if chat.AgentLocked && chat.AgentProfile.ID != "" {
			profiles = []AgentProfile{chat.AgentProfile}
			if currentProfile, ok := AgentProfileByID(state.agentProfiles, chat.AgentProvider); ok {
				profiles = []AgentProfile{currentProfile}
				chat.AgentProfile = currentProfile
			}
		}
		options := AgentProviderOptionsFromProfiles(profiles)
		normalizedProvider, normalizedModel, normalizedReasoning, err := NormalizeAgentSelection(provider, model, reasoning, options)
		if err != nil {
			return err
		}
		if chat.AgentLocked {
			if chat.AgentProvider != normalizedProvider {
				return fmt.Errorf("%w: agent 已锁定，不能切换 provider", ErrInvalidInput)
			}
		} else if profile, ok := AgentProfileByID(state.agentProfiles, normalizedProvider); ok {
			chat.AgentProfile = profile
		}
		chat.AgentProvider = normalizedProvider
		chat.AgentModel = normalizedModel
		chat.AgentReasoning = normalizedReasoning
		chat.ContextWindow = estimateChatContextWindowUsage(chat)
		chat.UpdatedAt = time.Now()
		state.chats[chatID] = chat
		state.lastAgent = LastAgentSelection{
			Provider:  normalizedProvider,
			Model:     normalizedModel,
			Reasoning: normalizedReasoning,
		}
		return nil
	}); err != nil {
		return Chat{}, err
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
	var chat Chat
	if err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok {
			return ErrNotFound
		}
		if !chat.AgentLocked {
			profile, ok := AgentProfileByID(state.agentProfiles, chat.AgentProvider)
			if !ok {
				return fmt.Errorf("%w: Profile 不存在: %s", ErrInvalidInput, chat.AgentProvider)
			}
			chat.AgentProfile = profile
		} else if currentProfile, ok := AgentProfileByID(state.agentProfiles, chat.AgentProvider); ok {
			chat.AgentProfile = currentProfile
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
		state.chats[chatID] = chat
		return nil
	}); err != nil {
		return Chat{}, ChatMessage{}, ChatMessage{}, err
	}
	return cloneChat(chat), userMessage, assistantMessage, nil
}

// AppendAssistantDelta 使用 chatID、messageID 和 delta 参数追加 assistant 流式文本。
func (s *Store) AppendAssistantDelta(chatID string, messageID string, delta string) (ChatMessage, bool) {
	if delta == "" {
		return ChatMessage{}, false
	}

	var message ChatMessage
	err := s.commit(func(state *storeState) error {
		chat, ok := state.chats[chatID]
		if !ok {
			return errStoreUnchanged
		}
		for index := range chat.Messages {
			item := &chat.Messages[index]
			if item.ID != messageID || item.Role != MessageRoleAssistant {
				continue
			}
			if item.Status != MessageStatusStreaming {
				return errStoreUnchanged
			}
			now := time.Now()
			item.Text += delta
			if len(item.Parts) > 0 && item.Parts[len(item.Parts)-1].Type == MessagePartTypeText {
				part := &item.Parts[len(item.Parts)-1]
				part.Text += delta
				part.UpdatedAt = now
			} else {
				item.Parts = append(item.Parts, MessagePart{
					ID:        newID("part"),
					Type:      MessagePartTypeText,
					Text:      delta,
					CreatedAt: now,
					UpdatedAt: now,
				})
			}
			item.UpdatedAt = now
			chat.ContextWindow = estimateChatContextWindowUsage(chat)
			chat.UpdatedAt = now
			state.chats[chatID] = chat
			message = *item
			return nil
		}
		return errStoreUnchanged
	})
	if err != nil {
		return ChatMessage{}, false
	}
	return message, true
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

	var chat Chat
	var toolMessage ChatMessage
	err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok {
			return errStoreUnchanged
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
					return errStoreUnchanged
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
			state.chats[chatID] = chat
			toolMessage = cloneChatMessage(*message)
			return nil
		}
		return errStoreUnchanged
	})
	if err != nil {
		return Chat{}, ChatMessage{}, false
	}
	return cloneChat(chat), toolMessage, true
}

// FinishAssistantMessage 使用 chatID、messageID 和 status 参数结束 assistant 消息。
func (s *Store) FinishAssistantMessage(chatID string, messageID string, status string) (Chat, ChatMessage, bool) {
	var chat Chat
	var message ChatMessage
	err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok {
			return errStoreUnchanged
		}
		for index := range chat.Messages {
			item := &chat.Messages[index]
			if item.ID != messageID || item.Role != MessageRoleAssistant {
				continue
			}
			message = *item
			if item.Status != MessageStatusStreaming {
				return errStoreUnchanged
			}
			now := time.Now()
			item.Status = status
			item.UpdatedAt = now
			if status == MessageStatusError {
				chat.Status = ChatStatusError
			} else {
				chat.Status = ChatStatusIdle
			}
			chat.UpdatedAt = now
			state.chats[chatID] = chat
			message = *item
			return nil
		}
		return errStoreUnchanged
	})
	if err != nil {
		if chat.ID != "" && message.ID != "" {
			return cloneChat(chat), message, false
		}
		return Chat{}, ChatMessage{}, false
	}
	return cloneChat(chat), message, true
}

// StopStreamingMessage 使用 chatID 和 status 参数停止聊天页中最后一条流式 assistant 消息。
func (s *Store) StopStreamingMessage(chatID string, status string) (Chat, ChatMessage, bool) {
	var chat Chat
	var message ChatMessage
	err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok {
			return errStoreUnchanged
		}
		for index := len(chat.Messages) - 1; index >= 0; index-- {
			item := &chat.Messages[index]
			if item.Role != MessageRoleAssistant || item.Status != MessageStatusStreaming {
				continue
			}
			now := time.Now()
			item.Status = status
			item.UpdatedAt = now
			chat.Status = ChatStatusIdle
			chat.UpdatedAt = now
			state.chats[chatID] = chat
			message = *item
			return nil
		}
		if chat.Status == ChatStatusRunning {
			chat.Status = ChatStatusIdle
			chat.UpdatedAt = time.Now()
			state.chats[chatID] = chat
			return nil
		}
		return errStoreUnchanged
	})
	if err != nil {
		if chat.ID != "" {
			return cloneChat(chat), ChatMessage{}, false
		}
		return Chat{}, ChatMessage{}, false
	}
	return cloneChat(chat), message, true
}

// AddSystemMessage 使用 chatID、text 和 status 参数追加系统消息。
func (s *Store) AddSystemMessage(chatID string, text string, status string) (Chat, ChatMessage, error) {
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
	var chat Chat
	if err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok {
			return ErrNotFound
		}
		chat.Messages = append(chat.Messages, message)
		chat.Status = ChatStatusError
		chat.UpdatedAt = now
		state.chats[chatID] = chat
		return nil
	}); err != nil {
		return Chat{}, ChatMessage{}, err
	}
	return cloneChat(chat), message, nil
}

// SetChatPlan 使用 chatID、messageID 和 text 参数记录当前待确认 plan。
func (s *Store) SetChatPlan(chatID string, messageID string, text string) (Chat, bool) {
	trimmedText := strings.TrimSpace(text)
	if trimmedText == "" {
		return Chat{}, false
	}
	var chat Chat
	err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok {
			return errStoreUnchanged
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
		state.chats[chatID] = chat
		return nil
	})
	if err != nil {
		return Chat{}, false
	}
	return cloneChat(chat), true
}

// MarkPlanExecuting 使用 chatID 和 planID 参数把 plan 标记为执行中并返回 plan 正文。
func (s *Store) MarkPlanExecuting(chatID string, planID string) (Chat, PlanApproval, error) {
	var chat Chat
	var plan PlanApproval
	if err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok {
			return ErrNotFound
		}
		if chat.Plan == nil || chat.Plan.ID != planID {
			return fmt.Errorf("%w: plan 不存在", ErrNotFound)
		}
		if chat.Plan.Status != "pending" {
			return fmt.Errorf("%w: plan 当前不可执行", ErrInvalidInput)
		}
		now := time.Now()
		chat.Plan.Status = "executing"
		chat.Plan.UpdatedAt = now
		chat.UpdatedAt = now
		state.chats[chatID] = chat
		plan = *chat.Plan
		return nil
	}); err != nil {
		return Chat{}, PlanApproval{}, err
	}
	return cloneChat(chat), plan, nil
}

// UpdateContextWindowUsage 使用 chatID 和 usage 参数更新 agent 上报的上下文窗口使用量。
func (s *Store) UpdateContextWindowUsage(chatID string, usage ContextWindowUsage) (Chat, bool) {
	var chat Chat
	err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok {
			return errStoreUnchanged
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
			return errStoreUnchanged
		}
		chat.ContextWindow = usage
		chat.UpdatedAt = time.Now()
		state.chats[chatID] = chat
		return nil
	})
	if err != nil {
		return Chat{}, false
	}
	return cloneChat(chat), true
}

// SetChatSessionID 使用 chatID 和 sessionID 参数记录 Claude 会话标识。
func (s *Store) SetChatSessionID(chatID string, sessionID string) (Chat, bool) {
	if strings.TrimSpace(sessionID) == "" {
		return Chat{}, false
	}
	var chat Chat
	err := s.commit(func(state *storeState) error {
		var ok bool
		chat, ok = state.chats[chatID]
		if !ok || chat.AgentSessionID == sessionID {
			return errStoreUnchanged
		}
		chat.AgentSessionID = sessionID
		chat.UpdatedAt = time.Now()
		state.chats[chatID] = chat
		return nil
	})
	if err != nil {
		return Chat{}, false
	}
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
	agentProfiles := cloneAgentProfiles(s.agentProfiles)
	agentProviders := AgentProviderOptionsFromProfiles(agentProfiles)
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
		AgentProfiles:      agentProfiles,
		BackendEnv:         BackendEnvSnapshot(),
		AgentSkills:        LoadAgentSkillOptions(projectPaths),
		LastAgentSelection: lastAgent,
	}
}

// AgentSkills 返回当前 project 和用户目录中可用的 skill 列表。
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
