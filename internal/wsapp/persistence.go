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
	persistedStoreSchemaVersion = 4
	// persistedStateFileName 表示状态文件名。
	persistedStateFileName = "state.json"
	// persistedTimelinesDirName 表示聊天 timeline 文件目录名。
	persistedTimelinesDirName = "timelines"
	// persistedChatTimelineSchemaVersion 表示聊天 timeline 文件格式版本。
	persistedChatTimelineSchemaVersion = 4
	// deferredTimelineSaveDelay 表示流式 timeline 延迟落盘等待时间。
	deferredTimelineSaveDelay = 750 * time.Millisecond
)

// PersistedStoreState 表示写入 state.json 的后端状态。
type PersistedStoreState struct {
	SchemaVersion      int                     `json:"schemaVersion"`      // SchemaVersion 表示持久化格式版本。
	Projects           []Project               `json:"projects"`           // Projects 表示 project 列表。
	Chats              []Chat                  `json:"chats"`              // Chats 表示聊天页摘要列表。
	AgentProfiles      []AgentProfile          `json:"agentProfiles"`      // AgentProfiles 表示可编辑 Profile 列表。
	LastAgentSelection LastAgentSelection      `json:"lastAgentSelection"` // LastAgentSelection 表示新聊天页默认 agent 配置。
	NextChatOrdinal    map[string]int          `json:"nextChatOrdinal"`    // NextChatOrdinal 表示每个 project 的聊天页序号。
	ChatTimelines      []PersistedChatTimeline `json:"-"`                  // ChatTimelines 表示需要写入独立文件的聊天 timeline。
}

// PersistedChatTimeline 表示写入单个聊天 timeline JSON 文件的内容。
type PersistedChatTimeline struct {
	SchemaVersion int               `json:"schemaVersion"` // SchemaVersion 表示 timeline 格式版本。
	ChatID        string            `json:"chatId"`        // ChatID 表示聊天页标识。
	Epoch         string            `json:"epoch"`         // Epoch 表示 timeline 世代标识。
	NextSeq       int64             `json:"nextSeq"`       // NextSeq 表示下一行序号。
	Rows          []ChatTimelineRow `json:"rows"`          // Rows 表示 append-only 行。
}

// PersistedStoreChange 表示一次 Store 提交产生的持久化变更集。
type PersistedStoreChange struct {
	State          PersistedStoreState `json:"-"` // State 表示提交后的完整 Store 状态。
	MetaDirty      bool                `json:"-"` // MetaDirty 表示 state.json 是否需要写入。
	DirtyChatIDs   []string            `json:"-"` // DirtyChatIDs 表示需要写入 timeline 文件的聊天页标识。
	DeletedChatIDs []string            `json:"-"` // DeletedChatIDs 表示需要删除 timeline 文件的聊天页标识。
	DeferTimelines bool                `json:"-"` // DeferTimelines 表示 timeline 是否允许延迟写入。
}

// JSONStorePersister 使用 JSON 文件保存 Store 状态。
type JSONStorePersister struct {
	statePath        string                           // statePath 表示 state.json 文件路径。
	mu               sync.Mutex                       // mu 保护延迟写入队列。
	pendingTimelines map[string]PersistedChatTimeline // pendingTimelines 表示等待延迟写入的聊天 timeline。
	pendingTimer     *time.Timer                      // pendingTimer 表示延迟写入计时器。
	lastErr          error                            // lastErr 表示后台延迟写入最近一次错误。
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

// Load 使用 agentProfiles 参数从 state.json 和聊天 timeline 文件读取 Store 状态。
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
	if persisted.SchemaVersion != persistedStoreSchemaVersion {
		return storeState{}, true, false, fmt.Errorf("不支持的状态文件版本: %d，请清理 AGENTHUB_DATA 后重新启动", persisted.SchemaVersion)
	}
	if err := p.loadPersistedChatTimelines(&persisted); err != nil {
		return storeState{}, true, false, err
	}
	state := storeStateFromPersistedState(persisted, agentProfiles)
	normalizeStoreState(&state)
	return state, true, false, nil
}

// SaveAll 使用 state 参数以原子替换方式写入 state.json 和所有聊天 timeline 文件。
func (p *JSONStorePersister) SaveAll(state PersistedStoreState) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopPendingTimerLocked()
	p.pendingTimelines = nil
	p.lastErr = nil
	return p.saveAllLocked(state)
}

// SaveChanges 使用 change 参数按变更集写入 state.json 和聊天 timeline 文件。
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

// Flush 使用 ctx 参数等待并写入延迟的聊天 timeline。
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
	if err := p.flushPendingTimelinesLocked(); err != nil {
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
	timelineDir := p.timelinesDir()
	if err := os.MkdirAll(timelineDir, 0700); err != nil {
		return fmt.Errorf("创建聊天 timeline 目录失败: %w", err)
	}
	expectedTimelineFiles := make(map[string]bool, len(state.ChatTimelines))
	for _, timeline := range state.ChatTimelines {
		if strings.TrimSpace(timeline.ChatID) == "" {
			return fmt.Errorf("聊天 timeline 缺少 chatID")
		}
		timeline = normalizePersistedChatTimeline(timeline)
		path := p.chatTimelinePath(timeline.ChatID)
		expectedTimelineFiles[filepath.Base(path)] = true
		if err := writeJSONAtomic(path, ".timeline-*.tmp", timeline); err != nil {
			return fmt.Errorf("写入聊天 timeline 文件失败: %w", err)
		}
	}
	if err := writeJSONAtomic(p.statePath, ".state-*.tmp", state); err != nil {
		return fmt.Errorf("写入状态文件失败: %w", err)
	}
	if err := p.removeStaleTimelineFiles(expectedTimelineFiles); err != nil {
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
	timelineDir := p.timelinesDir()
	if len(change.DirtyChatIDs) > 0 {
		if err := os.MkdirAll(timelineDir, 0700); err != nil {
			return fmt.Errorf("创建聊天 timeline 目录失败: %w", err)
		}
	}
	if change.MetaDirty {
		if err := writeJSONAtomic(p.statePath, ".state-*.tmp", state); err != nil {
			return fmt.Errorf("写入状态文件失败: %w", err)
		}
	}
	for _, chatID := range change.DeletedChatIDs {
		if p.pendingTimelines != nil {
			delete(p.pendingTimelines, chatID)
		}
		if err := os.Remove(p.chatTimelinePath(chatID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("删除聊天 timeline 文件失败: %w", err)
		}
	}
	timelinesByChatID := persistedChatTimelinesByID(state.ChatTimelines)
	immediateChatIDs := make([]string, 0, len(change.DirtyChatIDs))
	for _, chatID := range change.DirtyChatIDs {
		timeline, ok := timelinesByChatID[chatID]
		if !ok {
			return fmt.Errorf("聊天 timeline 不存在: %s", chatID)
		}
		if change.DeferTimelines {
			if p.pendingTimelines == nil {
				p.pendingTimelines = make(map[string]PersistedChatTimeline)
			}
			p.pendingTimelines[chatID] = normalizePersistedChatTimeline(timeline)
			continue
		}
		if p.pendingTimelines != nil {
			delete(p.pendingTimelines, chatID)
		}
		immediateChatIDs = append(immediateChatIDs, chatID)
	}
	for _, chatID := range immediateChatIDs {
		if err := p.writeChatTimelineLocked(normalizePersistedChatTimeline(timelinesByChatID[chatID])); err != nil {
			return err
		}
	}
	if change.DeferTimelines && len(change.DirtyChatIDs) > 0 {
		p.resetPendingTimerLocked()
	} else if len(p.pendingTimelines) == 0 {
		p.stopPendingTimerLocked()
	}
	return nil
}

// normalizePersistedChatTimeline 使用 timeline 参数生成可写入的聊天 timeline。
func normalizePersistedChatTimeline(timeline PersistedChatTimeline) PersistedChatTimeline {
	timeline.SchemaVersion = persistedChatTimelineSchemaVersion
	if strings.TrimSpace(timeline.Epoch) == "" {
		timeline.Epoch = newID("epoch")
	}
	if timeline.NextSeq <= 0 {
		timeline.NextSeq = nextTimelineSeq(timeline.Rows)
	}
	timeline.Rows = cloneChatTimelineRows(timeline.Rows)
	if timeline.Rows == nil {
		timeline.Rows = []ChatTimelineRow{}
	}
	return timeline
}

// writeChatTimelineLocked 使用 timeline 参数写入单个聊天 timeline 文件，调用方必须持有 p.mu。
func (p *JSONStorePersister) writeChatTimelineLocked(timeline PersistedChatTimeline) error {
	if strings.TrimSpace(timeline.ChatID) == "" {
		return fmt.Errorf("聊天 timeline 缺少 chatID")
	}
	if err := writeJSONAtomic(p.chatTimelinePath(timeline.ChatID), ".timeline-*.tmp", timeline); err != nil {
		return fmt.Errorf("写入聊天 timeline 文件失败: %w", err)
	}
	return nil
}

// resetPendingTimerLocked 重置延迟写入计时器，调用方必须持有 p.mu。
func (p *JSONStorePersister) resetPendingTimerLocked() {
	p.stopPendingTimerLocked()
	p.pendingTimer = time.AfterFunc(deferredTimelineSaveDelay, p.flushPendingTimelinesBackground)
}

// stopPendingTimerLocked 停止延迟写入计时器，调用方必须持有 p.mu。
func (p *JSONStorePersister) stopPendingTimerLocked() {
	if p.pendingTimer == nil {
		return
	}
	p.pendingTimer.Stop()
	p.pendingTimer = nil
}

// flushPendingTimelinesBackground 在后台写入延迟的聊天 timeline。
func (p *JSONStorePersister) flushPendingTimelinesBackground() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pendingTimer = nil
	if err := p.flushPendingTimelinesLocked(); err != nil {
		p.lastErr = err
	}
}

// flushPendingTimelinesLocked 写入全部延迟聊天 timeline，调用方必须持有 p.mu。
func (p *JSONStorePersister) flushPendingTimelinesLocked() error {
	if len(p.pendingTimelines) == 0 {
		return nil
	}
	chatIDs := make([]string, 0, len(p.pendingTimelines))
	for chatID := range p.pendingTimelines {
		chatIDs = append(chatIDs, chatID)
	}
	sort.Strings(chatIDs)
	if err := os.MkdirAll(p.timelinesDir(), 0700); err != nil {
		return fmt.Errorf("创建聊天 timeline 目录失败: %w", err)
	}
	for _, chatID := range chatIDs {
		if err := p.writeChatTimelineLocked(p.pendingTimelines[chatID]); err != nil {
			return err
		}
		delete(p.pendingTimelines, chatID)
	}
	return nil
}

// timelinesDir 返回聊天 timeline 文件目录。
func (p *JSONStorePersister) timelinesDir() string {
	return filepath.Join(filepath.Dir(p.statePath), persistedTimelinesDirName)
}

// chatTimelinePath 使用 chatID 参数返回聊天 timeline 文件路径。
func (p *JSONStorePersister) chatTimelinePath(chatID string) string {
	return filepath.Join(p.timelinesDir(), chatID+".json")
}

// loadPersistedChatTimelines 使用 persisted 参数加载独立 timeline 文件。
func (p *JSONStorePersister) loadPersistedChatTimelines(persisted *PersistedStoreState) error {
	timelines := make([]PersistedChatTimeline, 0, len(persisted.Chats))
	for index := range persisted.Chats {
		chat := &persisted.Chats[index]
		if strings.TrimSpace(chat.ID) == "" {
			continue
		}
		chat.Messages = []ChatMessage{}
		chat.Plan = nil
		data, err := os.ReadFile(p.chatTimelinePath(chat.ID))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				timeline := newChatTimelineState(chat.ID)
				timelines = append(timelines, persistedChatTimelineFromState(timeline))
				continue
			}
			return fmt.Errorf("读取聊天 timeline 文件失败: %w", err)
		}
		var timeline PersistedChatTimeline
		if err := json.Unmarshal(data, &timeline); err != nil {
			return fmt.Errorf("解析聊天 timeline 文件失败: %w", err)
		}
		if timeline.SchemaVersion != persistedChatTimelineSchemaVersion {
			return fmt.Errorf("不支持的聊天 timeline 文件版本: %d", timeline.SchemaVersion)
		}
		if strings.TrimSpace(timeline.ChatID) != "" && timeline.ChatID != chat.ID {
			return fmt.Errorf("聊天 timeline 文件 ID 不匹配: state=%s timeline=%s", chat.ID, timeline.ChatID)
		}
		timeline.ChatID = chat.ID
		timelines = append(timelines, normalizePersistedChatTimeline(timeline))
	}
	persisted.ChatTimelines = timelines
	return nil
}

// removeStaleTimelineFiles 使用 expected 参数移除已不存在聊天页的 timeline 文件。
func (p *JSONStorePersister) removeStaleTimelineFiles(expected map[string]bool) error {
	entries, err := os.ReadDir(p.timelinesDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("读取聊天 timeline 目录失败: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || expected[entry.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(p.timelinesDir(), entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("删除过期聊天 timeline 文件失败: %w", err)
		}
	}
	return nil
}

// normalizePersistedStateForSave 使用 state 参数拆出聊天摘要和 timeline。
func normalizePersistedStateForSave(state PersistedStoreState) PersistedStoreState {
	timelinesByChatID := persistedChatTimelinesByID(state.ChatTimelines)
	timelines := make([]PersistedChatTimeline, 0, len(state.Chats))
	for index := range state.Chats {
		chat := cloneChat(state.Chats[index])
		timeline, ok := timelinesByChatID[chat.ID]
		if !ok {
			timeline = persistedChatTimelineFromState(newChatTimelineState(chat.ID))
		}
		timeline.ChatID = chat.ID
		timelines = append(timelines, normalizePersistedChatTimeline(timeline))
		state.Chats[index] = cloneChatSummary(chat)
	}
	sort.Slice(timelines, func(i int, j int) bool {
		return timelines[i].ChatID < timelines[j].ChatID
	})
	state.ChatTimelines = timelines
	return state
}

// persistedChatTimelinesByID 使用 timelines 参数按聊天页标识索引 timeline。
func persistedChatTimelinesByID(timelines []PersistedChatTimeline) map[string]PersistedChatTimeline {
	result := make(map[string]PersistedChatTimeline, len(timelines))
	for _, timeline := range timelines {
		result[timeline.ChatID] = timeline
	}
	return result
}

// persistedChatTimelineFromState 使用 timeline 参数构造可写入独立文件的 timeline。
func persistedChatTimelineFromState(timeline chatTimelineState) PersistedChatTimeline {
	return PersistedChatTimeline{
		SchemaVersion: persistedChatTimelineSchemaVersion,
		ChatID:        timeline.ChatID,
		Epoch:         timeline.Epoch,
		NextSeq:       timeline.NextSeq,
		Rows:          cloneChatTimelineRows(timeline.Rows),
	}
}

// chatTimelineStateFromPersisted 使用 timeline 参数构造内存 timeline。
func chatTimelineStateFromPersisted(timeline PersistedChatTimeline) chatTimelineState {
	state := chatTimelineState{
		ChatID:  timeline.ChatID,
		Epoch:   timeline.Epoch,
		NextSeq: timeline.NextSeq,
		Rows:    cloneChatTimelineRows(timeline.Rows),
	}
	if strings.TrimSpace(state.Epoch) == "" {
		state.Epoch = newID("epoch")
	}
	if state.NextSeq <= 0 {
		state.NextSeq = nextTimelineSeq(state.Rows)
	}
	return state
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
	chatTimelines := make([]PersistedChatTimeline, 0, len(state.chats))
	for _, chat := range state.chats {
		chats = append(chats, cloneChatSummary(chat))
		timeline, ok := state.timelines[chat.ID]
		if !ok {
			timeline = newChatTimelineState(chat.ID)
		}
		chatTimelines = append(chatTimelines, persistedChatTimelineFromState(timeline))
	}
	sort.Slice(chats, func(i int, j int) bool {
		if chats[i].CreatedAt.Equal(chats[j].CreatedAt) {
			return chats[i].ID < chats[j].ID
		}
		return chats[i].CreatedAt.Before(chats[j].CreatedAt)
	})
	sort.Slice(chatTimelines, func(i int, j int) bool {
		return chatTimelines[i].ChatID < chatTimelines[j].ChatID
	})

	nextChatOrdinal := make(map[string]int, len(state.nextChatOrdinal))
	for projectID, ordinal := range state.nextChatOrdinal {
		nextChatOrdinal[projectID] = ordinal
	}
	return PersistedStoreState{
		SchemaVersion:      persistedStoreSchemaVersion,
		Projects:           projects,
		Chats:              chats,
		AgentProfiles:      cloneAgentProfiles(state.agentProfiles),
		LastAgentSelection: state.lastAgent,
		NextChatOrdinal:    nextChatOrdinal,
		ChatTimelines:      chatTimelines,
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
	timelines := make(map[string]chatTimelineState, len(persisted.ChatTimelines))
	for _, timeline := range persisted.ChatTimelines {
		if strings.TrimSpace(timeline.ChatID) == "" {
			continue
		}
		timelines[timeline.ChatID] = chatTimelineStateFromPersisted(timeline)
	}
	agentProfiles := persisted.AgentProfiles
	if len(agentProfiles) == 0 {
		agentProfiles = defaultAgentProfiles
	}
	nextChatOrdinal := make(map[string]int, len(persisted.NextChatOrdinal))
	for projectID, ordinal := range persisted.NextChatOrdinal {
		nextChatOrdinal[projectID] = ordinal
	}
	return storeState{
		projects:        projects,
		chats:           chats,
		timelines:       timelines,
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
		for _, message := range chat.Messages {
			if message.Status == MessageStatusStreaming {
				timeline := state.timelines[chatID]
				row := appendChatTimelineRow(&timeline, ChatTimelineItem{
					Type:      ChatTimelineItemMessageFinished,
					MessageID: message.ID,
					Status:    MessageStatusStopped,
				}, now)
				_ = row
				state.timelines[chatID] = timeline
				chatChanged = true
			}
		}
		if chatChanged {
			chat = projectChatFromTimeline(chat, state.timelines[chatID].Rows)
			state.chats[chatID] = chat
			changed = true
		}
	}
	return changed
}
