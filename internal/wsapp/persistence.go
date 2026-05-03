package wsapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	// persistedStoreSchemaVersion 表示当前 state.json 格式版本。
	persistedStoreSchemaVersion = 1
	// persistedStateFileName 表示状态文件名。
	persistedStateFileName = "state.json"
)

// PersistedStoreState 表示写入 state.json 的后端状态。
type PersistedStoreState struct {
	SchemaVersion      int                   `json:"schemaVersion"`      // SchemaVersion 表示持久化格式版本。
	Projects           []Project             `json:"projects"`           // Projects 表示 project 列表。
	Chats              []Chat                `json:"chats"`              // Chats 表示聊天页列表。
	AgentProviders     []AgentProviderOption `json:"agentProviders"`     // AgentProviders 表示可选 agent 和模型。
	LastAgentSelection LastAgentSelection    `json:"lastAgentSelection"` // LastAgentSelection 表示新聊天页默认 agent 配置。
	NextChatOrdinal    map[string]int        `json:"nextChatOrdinal"`    // NextChatOrdinal 表示每个 project 的聊天页序号。
}

// JSONStorePersister 使用 JSON 文件保存 Store 状态。
type JSONStorePersister struct {
	statePath string // statePath 表示 state.json 文件路径。
}

// NewPersistentStore 使用 dataDir 和 agentProviders 参数创建带 JSON 持久化的 Store。
func NewPersistentStore(dataDir string, agentProviders []AgentProviderOption) (*Store, error) {
	if len(agentProviders) == 0 {
		agentProviders = DefaultAgentProviderOptions()
	}
	persister := NewJSONStorePersister(dataDir)
	state, existed, err := persister.Load(agentProviders)
	if err != nil {
		return nil, err
	}
	changed := normalizeLoadedRuntimeState(&state)
	if existed && changed {
		if err := persister.Save(persistedStateFromStoreState(state)); err != nil {
			return nil, err
		}
	}
	return newStoreFromState(state, persister), nil
}

// NewJSONStorePersister 使用 dataDir 参数创建 JSON 持久化器。
func NewJSONStorePersister(dataDir string) *JSONStorePersister {
	return &JSONStorePersister{statePath: filepath.Join(dataDir, persistedStateFileName)}
}

// StatePath 返回当前持久化器使用的 state.json 路径。
func (p *JSONStorePersister) StatePath() string {
	return p.statePath
}

// Load 使用 agentProviders 参数从 state.json 读取 Store 状态。
func (p *JSONStorePersister) Load(agentProviders []AgentProviderOption) (storeState, bool, error) {
	data, err := os.ReadFile(p.statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storeState{
				projects:        make(map[string]Project),
				chats:           make(map[string]Chat),
				nextChatOrdinal: make(map[string]int),
				agentProviders:  cloneAgentProviderOptions(agentProviders),
				lastAgent:       defaultLastAgentSelection(agentProviders),
			}, false, nil
		}
		return storeState{}, false, fmt.Errorf("读取状态文件失败: %w", err)
	}
	var persisted PersistedStoreState
	if err := json.Unmarshal(data, &persisted); err != nil {
		return storeState{}, true, fmt.Errorf("解析状态文件失败: %w", err)
	}
	if persisted.SchemaVersion != 0 && persisted.SchemaVersion != persistedStoreSchemaVersion {
		return storeState{}, true, fmt.Errorf("不支持的状态文件版本: %d", persisted.SchemaVersion)
	}
	state := storeStateFromPersistedState(persisted, agentProviders)
	normalizeStoreState(&state)
	return state, true, nil
}

// Save 使用 state 参数以原子替换方式写入 state.json。
func (p *JSONStorePersister) Save(state PersistedStoreState) error {
	state.SchemaVersion = persistedStoreSchemaVersion
	if err := os.MkdirAll(filepath.Dir(p.statePath), 0700); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("编码状态文件失败: %w", err)
	}
	data = append(data, '\n')

	tempFile, err := os.CreateTemp(filepath.Dir(p.statePath), ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时状态文件失败: %w", err)
	}
	tempPath := tempFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tempFile.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := tempFile.Chmod(0600); err != nil {
		return fmt.Errorf("设置临时状态文件权限失败: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("写入临时状态文件失败: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("同步临时状态文件失败: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		closed = true
		return fmt.Errorf("关闭临时状态文件失败: %w", err)
	}
	closed = true
	if err := os.Rename(tempPath, p.statePath); err != nil {
		return fmt.Errorf("替换状态文件失败: %w", err)
	}
	return nil
}

// persistedStateFromStoreState 使用 state 参数生成可写入 JSON 的状态。
func persistedStateFromStoreState(state storeState) PersistedStoreState {
	projects := make([]Project, 0, len(state.projects))
	for _, project := range state.projects {
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i int, j int) bool {
		if projects[i].CreatedAt.Equal(projects[j].CreatedAt) {
			return projects[i].ID < projects[j].ID
		}
		return projects[i].CreatedAt.Before(projects[j].CreatedAt)
	})

	chats := make([]Chat, 0, len(state.chats))
	for _, chat := range state.chats {
		chats = append(chats, cloneChat(chat))
	}
	sort.Slice(chats, func(i int, j int) bool {
		if chats[i].CreatedAt.Equal(chats[j].CreatedAt) {
			return chats[i].ID < chats[j].ID
		}
		return chats[i].CreatedAt.Before(chats[j].CreatedAt)
	})

	nextChatOrdinal := make(map[string]int, len(state.nextChatOrdinal))
	for projectID, ordinal := range state.nextChatOrdinal {
		nextChatOrdinal[projectID] = ordinal
	}
	return PersistedStoreState{
		SchemaVersion:      persistedStoreSchemaVersion,
		Projects:           projects,
		Chats:              chats,
		AgentProviders:     cloneAgentProviderOptions(state.agentProviders),
		LastAgentSelection: state.lastAgent,
		NextChatOrdinal:    nextChatOrdinal,
	}
}

// storeStateFromPersistedState 使用 persisted 和 defaultAgentProviders 参数构造内存状态。
func storeStateFromPersistedState(persisted PersistedStoreState, defaultAgentProviders []AgentProviderOption) storeState {
	projects := make(map[string]Project, len(persisted.Projects))
	for _, project := range persisted.Projects {
		projects[project.ID] = project
	}
	chats := make(map[string]Chat, len(persisted.Chats))
	for _, chat := range persisted.Chats {
		chats[chat.ID] = cloneChat(chat)
	}
	agentProviders := persisted.AgentProviders
	if len(agentProviders) == 0 {
		agentProviders = defaultAgentProviders
	}
	nextChatOrdinal := make(map[string]int, len(persisted.NextChatOrdinal))
	for projectID, ordinal := range persisted.NextChatOrdinal {
		nextChatOrdinal[projectID] = ordinal
	}
	return storeState{
		projects:        projects,
		chats:           chats,
		nextChatOrdinal: nextChatOrdinal,
		agentProviders:  cloneAgentProviderOptions(agentProviders),
		lastAgent:       persisted.LastAgentSelection,
	}
}

// normalizeLoadedRuntimeState 使用 state 参数把无法跨进程恢复的运行中状态改为停止。
func normalizeLoadedRuntimeState(state *storeState) bool {
	changed := false
	now := time.Now()
	for chatID, chat := range state.chats {
		chatChanged := false
		if chat.Status == ChatStatusRunning {
			chat.Status = ChatStatusIdle
			chat.UpdatedAt = now
			chatChanged = true
		}
		for index := range chat.Messages {
			message := &chat.Messages[index]
			if message.Status != MessageStatusStreaming {
				continue
			}
			message.Status = MessageStatusStopped
			message.UpdatedAt = now
			chat.UpdatedAt = now
			chatChanged = true
		}
		if chatChanged {
			state.chats[chatID] = chat
			changed = true
		}
	}
	return changed
}
