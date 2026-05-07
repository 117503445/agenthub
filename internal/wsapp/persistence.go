package wsapp

import (
	"context"
	"encoding/json"
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
	// persistedStoreSchemaVersion 表示当前 state.json 格式版本。
	persistedStoreSchemaVersion = 3
	// persistedStateFileName 表示状态文件名。
	persistedStateFileName = "state.json"
	// persistedChatDetailsDirName 表示聊天详情文件目录名。
	persistedChatDetailsDirName = "chats"
	// persistedChatDetailSchemaVersion 表示聊天详情文件格式版本。
	persistedChatDetailSchemaVersion = 1
	// deferredChatDetailSaveDelay 表示流式详情延迟落盘等待时间。
	deferredChatDetailSaveDelay = 750 * time.Millisecond
)

// PersistedStoreState 表示写入 state.json 的后端状态。
type PersistedStoreState struct {
	SchemaVersion      int                   `json:"schemaVersion"`      // SchemaVersion 表示持久化格式版本。
	Projects           []Project             `json:"projects"`           // Projects 表示 project 列表。
	Chats              []Chat                `json:"chats"`              // Chats 表示聊天页摘要列表。
	AgentProviders     []AgentProviderOption `json:"agentProviders"`     // AgentProviders 表示可选 agent 和模型。
	AgentProfiles      []AgentProfile        `json:"agentProfiles"`      // AgentProfiles 表示可编辑 Profile 列表。
	LastAgentSelection LastAgentSelection    `json:"lastAgentSelection"` // LastAgentSelection 表示新聊天页默认 agent 配置。
	NextChatOrdinal    map[string]int        `json:"nextChatOrdinal"`    // NextChatOrdinal 表示每个 project 的聊天页序号。
	ChatDetails        []PersistedChatDetail `json:"-"`                  // ChatDetails 表示需要写入独立文件的聊天详情。
}

// PersistedChatDetail 表示写入单个聊天详情 JSON 文件的内容。
type PersistedChatDetail struct {
	SchemaVersion int           `json:"schemaVersion"`  // SchemaVersion 表示聊天详情格式版本。
	ChatID        string        `json:"chatId"`         // ChatID 表示聊天页标识。
	Plan          *PlanApproval `json:"plan,omitempty"` // Plan 表示当前聊天页的 plan 详情。
	Messages      []ChatMessage `json:"messages"`       // Messages 表示聊天消息列表。
}

// PersistedStoreChange 表示一次 Store 提交产生的持久化变更集。
type PersistedStoreChange struct {
	State            PersistedStoreState `json:"-"` // State 表示提交后的完整 Store 状态。
	MetaDirty        bool                `json:"-"` // MetaDirty 表示 state.json 是否需要写入。
	DirtyChatIDs     []string            `json:"-"` // DirtyChatIDs 表示需要写入详情文件的聊天页标识。
	DeletedChatIDs   []string            `json:"-"` // DeletedChatIDs 表示需要删除详情文件的聊天页标识。
	DeferChatDetails bool                `json:"-"` // DeferChatDetails 表示聊天详情是否允许延迟写入。
}

// JSONStorePersister 使用 JSON 文件保存 Store 状态。
type JSONStorePersister struct {
	statePath          string                         // statePath 表示 state.json 文件路径。
	mu                 sync.Mutex                     // mu 保护延迟写入队列。
	pendingChatDetails map[string]PersistedChatDetail // pendingChatDetails 表示等待延迟写入的聊天详情。
	pendingTimer       *time.Timer                    // pendingTimer 表示延迟写入计时器。
	lastErr            error                          // lastErr 表示后台延迟写入最近一次错误。
}

// NewPersistentStore 使用 dataDir 和 agentProfiles 参数创建带 JSON 持久化的 Store。
func NewPersistentStore(dataDir string, agentProfiles []AgentProfile) (*Store, error) {
	if len(agentProfiles) == 0 {
		agentProfiles = AgentProfiles(AgentOptionsConfig{})
	}
	persister := NewJSONStorePersister(dataDir)
	state, existed, needsSave, err := persister.Load(agentProfiles)
	if err != nil {
		return nil, err
	}
	changed := normalizeLoadedRuntimeState(&state)
	if existed && (changed || needsSave) {
		if err := persister.SaveAll(persistedStateFromStoreState(state)); err != nil {
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

// Load 使用 agentProfiles 参数从 state.json 和聊天详情文件读取 Store 状态。
func (p *JSONStorePersister) Load(agentProfiles []AgentProfile) (storeState, bool, bool, error) {
	data, err := os.ReadFile(p.statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storeState{
				projects:        make(map[string]Project),
				chats:           make(map[string]Chat),
				nextChatOrdinal: make(map[string]int),
				agentProfiles:   cloneAgentProfiles(agentProfiles),
				lastAgent:       defaultLastAgentSelection(AgentProviderOptionsFromProfiles(agentProfiles)),
			}, false, false, nil
		}
		return storeState{}, false, false, fmt.Errorf("读取状态文件失败: %w", err)
	}
	var persisted PersistedStoreState
	if err := json.Unmarshal(data, &persisted); err != nil {
		return storeState{}, true, false, fmt.Errorf("解析状态文件失败: %w", err)
	}
	if persisted.SchemaVersion != 0 && persisted.SchemaVersion != 1 && persisted.SchemaVersion != 2 && persisted.SchemaVersion != persistedStoreSchemaVersion {
		return storeState{}, true, false, fmt.Errorf("不支持的状态文件版本: %d", persisted.SchemaVersion)
	}
	if persisted.SchemaVersion >= 3 {
		if err := p.loadPersistedChatDetails(&persisted); err != nil {
			return storeState{}, true, false, err
		}
	}
	state := storeStateFromPersistedState(persisted, agentProfiles)
	normalizeStoreState(&state)
	return state, true, persisted.SchemaVersion < persistedStoreSchemaVersion, nil
}

// SaveAll 使用 state 参数以原子替换方式写入 state.json 和所有聊天详情文件。
func (p *JSONStorePersister) SaveAll(state PersistedStoreState) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopPendingTimerLocked()
	p.pendingChatDetails = nil
	p.lastErr = nil
	return p.saveAllLocked(state)
}

// SaveChanges 使用 change 参数按变更集写入 state.json 和聊天详情文件。
func (p *JSONStorePersister) SaveChanges(change PersistedStoreChange) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastErr != nil {
		err := p.lastErr
		p.lastErr = nil
		return err
	}
	return p.saveChangesLocked(change)
}

// Flush 使用 ctx 参数等待并写入延迟的聊天详情。
func (p *JSONStorePersister) Flush(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopPendingTimerLocked()
	if p.lastErr != nil {
		err := p.lastErr
		p.lastErr = nil
		return err
	}
	if err := p.flushPendingChatDetailsLocked(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// saveAllLocked 使用 state 参数写入完整状态，调用方必须持有 p.mu。
func (p *JSONStorePersister) saveAllLocked(state PersistedStoreState) error {
	state = normalizePersistedStateForSave(state)
	state.SchemaVersion = persistedStoreSchemaVersion
	if err := os.MkdirAll(filepath.Dir(p.statePath), 0700); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}
	detailDir := p.chatDetailsDir()
	if err := os.MkdirAll(detailDir, 0700); err != nil {
		return fmt.Errorf("创建聊天详情目录失败: %w", err)
	}
	expectedDetailFiles := make(map[string]bool, len(state.ChatDetails))
	for _, detail := range state.ChatDetails {
		if strings.TrimSpace(detail.ChatID) == "" {
			return fmt.Errorf("聊天详情缺少 chatID")
		}
		detail.SchemaVersion = persistedChatDetailSchemaVersion
		detail.Messages = cloneChatMessages(detail.Messages)
		if detail.Messages == nil {
			detail.Messages = []ChatMessage{}
		}
		path := p.chatDetailPath(detail.ChatID)
		expectedDetailFiles[filepath.Base(path)] = true
		if err := writeJSONAtomic(path, ".chat-*.tmp", detail); err != nil {
			return fmt.Errorf("写入聊天详情文件失败: %w", err)
		}
	}
	if err := writeJSONAtomic(p.statePath, ".state-*.tmp", state); err != nil {
		return fmt.Errorf("写入状态文件失败: %w", err)
	}
	if err := p.removeStaleChatDetailFiles(expectedDetailFiles); err != nil {
		return err
	}
	return nil
}

// saveChangesLocked 使用 change 参数写入变更集，调用方必须持有 p.mu。
func (p *JSONStorePersister) saveChangesLocked(change PersistedStoreChange) error {
	state := normalizePersistedStateForSave(change.State)
	state.SchemaVersion = persistedStoreSchemaVersion
	if err := os.MkdirAll(filepath.Dir(p.statePath), 0700); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}
	detailDir := p.chatDetailsDir()
	if len(change.DirtyChatIDs) > 0 {
		if err := os.MkdirAll(detailDir, 0700); err != nil {
			return fmt.Errorf("创建聊天详情目录失败: %w", err)
		}
	}
	if change.MetaDirty {
		if err := writeJSONAtomic(p.statePath, ".state-*.tmp", state); err != nil {
			return fmt.Errorf("写入状态文件失败: %w", err)
		}
	}
	for _, chatID := range change.DeletedChatIDs {
		if p.pendingChatDetails != nil {
			delete(p.pendingChatDetails, chatID)
		}
		if err := os.Remove(p.chatDetailPath(chatID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("删除聊天详情文件失败: %w", err)
		}
	}
	detailsByChatID := persistedChatDetailsByID(state.ChatDetails)
	immediateChatIDs := make([]string, 0, len(change.DirtyChatIDs))
	for _, chatID := range change.DirtyChatIDs {
		detail, ok := detailsByChatID[chatID]
		if !ok {
			return fmt.Errorf("聊天详情不存在: %s", chatID)
		}
		if change.DeferChatDetails {
			if p.pendingChatDetails == nil {
				p.pendingChatDetails = make(map[string]PersistedChatDetail)
			}
			p.pendingChatDetails[chatID] = normalizePersistedChatDetail(detail)
			continue
		}
		if p.pendingChatDetails != nil {
			delete(p.pendingChatDetails, chatID)
		}
		immediateChatIDs = append(immediateChatIDs, chatID)
	}
	for _, chatID := range immediateChatIDs {
		if err := p.writeChatDetailLocked(normalizePersistedChatDetail(detailsByChatID[chatID])); err != nil {
			return err
		}
	}
	if change.DeferChatDetails && len(change.DirtyChatIDs) > 0 {
		p.resetPendingTimerLocked()
	} else if len(p.pendingChatDetails) == 0 {
		p.stopPendingTimerLocked()
	}
	return nil
}

// normalizePersistedChatDetail 使用 detail 参数生成可写入的聊天详情。
func normalizePersistedChatDetail(detail PersistedChatDetail) PersistedChatDetail {
	detail.SchemaVersion = persistedChatDetailSchemaVersion
	detail.Messages = cloneChatMessages(detail.Messages)
	if detail.Messages == nil {
		detail.Messages = []ChatMessage{}
	}
	if detail.Plan != nil {
		plan := *detail.Plan
		detail.Plan = &plan
	}
	return detail
}

// writeChatDetailLocked 使用 detail 参数写入单个聊天详情文件，调用方必须持有 p.mu。
func (p *JSONStorePersister) writeChatDetailLocked(detail PersistedChatDetail) error {
	if strings.TrimSpace(detail.ChatID) == "" {
		return fmt.Errorf("聊天详情缺少 chatID")
	}
	if err := writeJSONAtomic(p.chatDetailPath(detail.ChatID), ".chat-*.tmp", detail); err != nil {
		return fmt.Errorf("写入聊天详情文件失败: %w", err)
	}
	return nil
}

// resetPendingTimerLocked 重置延迟写入计时器，调用方必须持有 p.mu。
func (p *JSONStorePersister) resetPendingTimerLocked() {
	p.stopPendingTimerLocked()
	p.pendingTimer = time.AfterFunc(deferredChatDetailSaveDelay, p.flushPendingChatDetailsBackground)
}

// stopPendingTimerLocked 停止延迟写入计时器，调用方必须持有 p.mu。
func (p *JSONStorePersister) stopPendingTimerLocked() {
	if p.pendingTimer == nil {
		return
	}
	p.pendingTimer.Stop()
	p.pendingTimer = nil
}

// flushPendingChatDetailsBackground 在后台写入延迟的聊天详情。
func (p *JSONStorePersister) flushPendingChatDetailsBackground() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pendingTimer = nil
	if err := p.flushPendingChatDetailsLocked(); err != nil {
		p.lastErr = err
	}
}

// flushPendingChatDetailsLocked 写入全部延迟聊天详情，调用方必须持有 p.mu。
func (p *JSONStorePersister) flushPendingChatDetailsLocked() error {
	if len(p.pendingChatDetails) == 0 {
		return nil
	}
	chatIDs := make([]string, 0, len(p.pendingChatDetails))
	for chatID := range p.pendingChatDetails {
		chatIDs = append(chatIDs, chatID)
	}
	sort.Strings(chatIDs)
	if err := os.MkdirAll(p.chatDetailsDir(), 0700); err != nil {
		return fmt.Errorf("创建聊天详情目录失败: %w", err)
	}
	for _, chatID := range chatIDs {
		if err := p.writeChatDetailLocked(p.pendingChatDetails[chatID]); err != nil {
			return err
		}
		delete(p.pendingChatDetails, chatID)
	}
	return nil
}

// chatDetailsDir 返回聊天详情文件目录。
func (p *JSONStorePersister) chatDetailsDir() string {
	return filepath.Join(filepath.Dir(p.statePath), persistedChatDetailsDirName)
}

// chatDetailPath 使用 chatID 参数返回聊天详情文件路径。
func (p *JSONStorePersister) chatDetailPath(chatID string) string {
	return filepath.Join(p.chatDetailsDir(), chatID+".json")
}

// loadPersistedChatDetails 使用 persisted 参数把独立详情文件合并回聊天页。
func (p *JSONStorePersister) loadPersistedChatDetails(persisted *PersistedStoreState) error {
	for index := range persisted.Chats {
		chat := &persisted.Chats[index]
		if strings.TrimSpace(chat.ID) == "" {
			continue
		}
		data, err := os.ReadFile(p.chatDetailPath(chat.ID))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				chat.Messages = []ChatMessage{}
				chat.Plan = nil
				continue
			}
			return fmt.Errorf("读取聊天详情文件失败: %w", err)
		}
		var detail PersistedChatDetail
		if err := json.Unmarshal(data, &detail); err != nil {
			return fmt.Errorf("解析聊天详情文件失败: %w", err)
		}
		if detail.SchemaVersion != 0 && detail.SchemaVersion != persistedChatDetailSchemaVersion {
			return fmt.Errorf("不支持的聊天详情文件版本: %d", detail.SchemaVersion)
		}
		if strings.TrimSpace(detail.ChatID) != "" && detail.ChatID != chat.ID {
			return fmt.Errorf("聊天详情文件 ID 不匹配: state=%s detail=%s", chat.ID, detail.ChatID)
		}
		chat.Messages = cloneChatMessages(detail.Messages)
		if chat.Messages == nil {
			chat.Messages = []ChatMessage{}
		}
		if detail.Plan != nil {
			plan := *detail.Plan
			chat.Plan = &plan
		} else {
			chat.Plan = nil
		}
	}
	return nil
}

// removeStaleChatDetailFiles 使用 expected 参数移除已不存在聊天页的详情文件。
func (p *JSONStorePersister) removeStaleChatDetailFiles(expected map[string]bool) error {
	entries, err := os.ReadDir(p.chatDetailsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("读取聊天详情目录失败: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || expected[entry.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(p.chatDetailsDir(), entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("删除过期聊天详情文件失败: %w", err)
		}
	}
	return nil
}

// normalizePersistedStateForSave 使用 state 参数拆出聊天摘要和详情。
func normalizePersistedStateForSave(state PersistedStoreState) PersistedStoreState {
	detailsByChatID := make(map[string]PersistedChatDetail, len(state.ChatDetails))
	for _, detail := range state.ChatDetails {
		detailsByChatID[detail.ChatID] = detail
	}
	details := make([]PersistedChatDetail, 0, len(state.Chats))
	for index := range state.Chats {
		chat := cloneChat(state.Chats[index])
		detail, ok := detailsByChatID[chat.ID]
		if !ok {
			detail = persistedChatDetailFromChat(chat)
		}
		detail.ChatID = chat.ID
		if detail.Messages == nil {
			detail.Messages = []ChatMessage{}
		}
		details = append(details, detail)
		state.Chats[index] = cloneChatSummary(chat)
	}
	sort.Slice(details, func(i int, j int) bool {
		return details[i].ChatID < details[j].ChatID
	})
	state.ChatDetails = details
	return state
}

// persistedChatDetailFromChat 使用 chat 参数构造可写入独立文件的聊天详情。
func persistedChatDetailFromChat(chat Chat) PersistedChatDetail {
	var plan *PlanApproval
	if chat.Plan != nil {
		copiedPlan := *chat.Plan
		plan = &copiedPlan
	}
	messages := cloneChatMessages(chat.Messages)
	if messages == nil {
		messages = []ChatMessage{}
	}
	return PersistedChatDetail{
		SchemaVersion: persistedChatDetailSchemaVersion,
		ChatID:        chat.ID,
		Plan:          plan,
		Messages:      messages,
	}
}

// writeJSONAtomic 使用 path、tempPattern 和 value 参数原子写入 JSON 文件。
func writeJSONAtomic(path string, tempPattern string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 JSON 失败: %w", err)
	}
	data = append(data, '\n')

	tempFile, err := os.CreateTemp(filepath.Dir(path), tempPattern)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
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
		return fmt.Errorf("设置临时文件权限失败: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("同步临时文件失败: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		closed = true
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	closed = true
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("替换文件失败: %w", err)
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
	chatDetails := make([]PersistedChatDetail, 0, len(state.chats))
	for _, chat := range state.chats {
		chats = append(chats, cloneChatSummary(chat))
		chatDetails = append(chatDetails, persistedChatDetailFromChat(chat))
	}
	sort.Slice(chats, func(i int, j int) bool {
		if chats[i].CreatedAt.Equal(chats[j].CreatedAt) {
			return chats[i].ID < chats[j].ID
		}
		return chats[i].CreatedAt.Before(chats[j].CreatedAt)
	})
	sort.Slice(chatDetails, func(i int, j int) bool {
		return chatDetails[i].ChatID < chatDetails[j].ChatID
	})

	nextChatOrdinal := make(map[string]int, len(state.nextChatOrdinal))
	for projectID, ordinal := range state.nextChatOrdinal {
		nextChatOrdinal[projectID] = ordinal
	}
	return PersistedStoreState{
		SchemaVersion:      persistedStoreSchemaVersion,
		Projects:           projects,
		Chats:              chats,
		AgentProviders:     AgentProviderOptionsFromProfiles(state.agentProfiles),
		AgentProfiles:      cloneAgentProfiles(state.agentProfiles),
		LastAgentSelection: state.lastAgent,
		NextChatOrdinal:    nextChatOrdinal,
		ChatDetails:        chatDetails,
	}
}

// storeStateFromPersistedState 使用 persisted 和 defaultAgentProfiles 参数构造内存状态。
func storeStateFromPersistedState(persisted PersistedStoreState, defaultAgentProfiles []AgentProfile) storeState {
	projects := make(map[string]Project, len(persisted.Projects))
	for _, project := range persisted.Projects {
		projects[project.ID] = project
	}
	chats := make(map[string]Chat, len(persisted.Chats))
	for _, chat := range persisted.Chats {
		chats[chat.ID] = cloneChat(chat)
	}
	agentProfiles := persisted.AgentProfiles
	if persisted.SchemaVersion < 2 && len(agentProfiles) == 0 && len(persisted.AgentProviders) > 0 {
		agentProfiles = AgentProfilesFromProviderOptions(persisted.AgentProviders)
	}
	if persisted.SchemaVersion < 2 && len(agentProfiles) == 0 {
		agentProfiles = defaultAgentProfiles
	}
	nextChatOrdinal := make(map[string]int, len(persisted.NextChatOrdinal))
	for projectID, ordinal := range persisted.NextChatOrdinal {
		nextChatOrdinal[projectID] = ordinal
	}
	return storeState{
		projects:        projects,
		chats:           chats,
		nextChatOrdinal: nextChatOrdinal,
		agentProfiles:   cloneAgentProfiles(agentProfiles),
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
