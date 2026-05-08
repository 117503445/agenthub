package wsapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// failingStorePersister 表示始终保存失败的测试持久化器。
type failingStorePersister struct{}

// SaveAll 使用 state 参数模拟完整保存失败。
func (f failingStorePersister) SaveAll(state PersistedStoreState) error {
	return errors.New("forced save failure")
}

// SaveChanges 使用 change 参数模拟增量保存失败。
func (f failingStorePersister) SaveChanges(change PersistedStoreChange) error {
	return errors.New("forced save failure")
}

// Flush 使用 ctx 参数模拟无延迟写入。
func (f failingStorePersister) Flush(ctx context.Context) error {
	return nil
}

// TestPersistentStoreSaveLoad 验证 Store 状态和 timeline 可保存并恢复。
func TestPersistentStoreSaveLoad(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewPersistentStore(dataDir, AgentProfiles(AgentOptionsConfig{}))
	if err != nil {
		t.Fatalf("创建持久化 Store 失败: %v", err)
	}
	project, err := store.CreateProject(t.TempDir())
	if err != nil {
		t.Fatalf("创建 project 失败: %v", err)
	}
	chat, err := store.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("创建聊天页失败: %v", err)
	}
	if _, err := store.AddAgentModel(AgentProviderClaudeCode, "claude-custom-test"); err != nil {
		t.Fatalf("新增模型失败: %v", err)
	}
	chat, err = store.UpdateChatAgent(chat.ID, AgentProviderCodex, "gpt-5.5", "xhigh")
	if err != nil {
		t.Fatalf("更新 agent 失败: %v", err)
	}
	if _, _, assistant, _, err := store.AddRunMessages(chat.ID, "持久化测试", nil, false); err != nil {
		t.Fatalf("追加运行消息失败: %v", err)
	} else {
		if _, _, ok := store.AppendAssistantDelta(chat.ID, assistant.ID, "完成"); !ok {
			t.Fatalf("追加 assistant 输出失败")
		}
		if _, _, _, ok := store.FinishAssistantMessage(chat.ID, assistant.ID, MessageStatusComplete); !ok {
			t.Fatalf("结束 assistant 输出失败")
		}
	}
	secondChat, err := store.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("创建第二个聊天页失败: %v", err)
	}
	if _, changed, err := store.UpdateChatDraft(secondChat.ID, "持久化草稿"); err != nil {
		t.Fatalf("保存第二个聊天页草稿失败: %v", err)
	} else if !changed {
		t.Fatalf("第二个聊天页草稿应该发生变化")
	}
	assertPersistentTimeline(t, dataDir, chat.ID, "持久化测试")

	loaded, err := NewPersistentStore(dataDir, AgentProfiles(AgentOptionsConfig{}))
	if err != nil {
		t.Fatalf("加载持久化 Store 失败: %v", err)
	}
	snapshot := loaded.Snapshot()
	if len(snapshot.Projects) != 1 || len(snapshot.Chats) != 2 {
		t.Fatalf("恢复后的状态数量不正确: %#v", snapshot)
	}
	if !agentOptionsContainModel(snapshot.AgentProviders, AgentProviderClaudeCode, "claude-custom-test") {
		t.Fatalf("恢复后的自定义模型缺失: %#v", snapshot.AgentProviders)
	}
	if restored := findChatByID(snapshot.Chats, secondChat.ID); restored == nil || restored.DraftText != "持久化草稿" {
		t.Fatalf("恢复后的聊天页草稿不正确: %#v", snapshot.Chats)
	}
	restoredChat, err := loaded.GetChat(chat.ID)
	if err != nil {
		t.Fatalf("读取恢复聊天 timeline 投影失败: %v", err)
	}
	if len(restoredChat.Messages) != 2 || !strings.Contains(restoredChat.Messages[1].Text, "完成") {
		t.Fatalf("恢复后的 timeline 投影不正确: %#v", restoredChat.Messages)
	}
	nextChat, err := loaded.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("恢复后创建聊天页失败: %v", err)
	}
	if nextChat.Title != "聊天 3" {
		t.Fatalf("恢复后的聊天页序号不正确: %q", nextChat.Title)
	}
}

// TestPersistentStoreNormalizesRunningChatAsRecoverable 验证重启后运行中聊天会停止输出但保留恢复句柄。
func TestPersistentStoreNormalizesRunningChatAsRecoverable(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewPersistentStore(dataDir, AgentProfiles(AgentOptionsConfig{EnableMockAgent: true}))
	if err != nil {
		t.Fatalf("创建持久化 Store 失败: %v", err)
	}
	project, err := store.CreateProject(t.TempDir())
	if err != nil {
		t.Fatalf("创建 project 失败: %v", err)
	}
	chat, err := store.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("创建聊天页失败: %v", err)
	}
	chat, err = store.UpdateChatAgent(chat.ID, AgentProviderMockCodex, "mock-codex-gpt-5.5", "xhigh")
	if err != nil {
		t.Fatalf("切换 mock Codex 失败: %v", err)
	}
	if _, _, assistant, _, err := store.AddRunMessages(chat.ID, "运行中恢复", nil, false); err != nil {
		t.Fatalf("追加运行消息失败: %v", err)
	} else if _, _, ok := store.AppendAssistantDelta(chat.ID, assistant.ID, "partial"); !ok {
		t.Fatalf("追加 partial 输出失败")
	}
	if updatedChat, ok := store.SetChatPersistenceSessionID(chat.ID, "thread-recoverable"); !ok || updatedChat.AgentPersistence == nil {
		t.Fatalf("保存恢复句柄失败: ok=%v chat=%#v", ok, updatedChat)
	}
	if err := store.Flush(context.Background()); err != nil {
		t.Fatalf("刷新持久化失败: %v", err)
	}

	loaded, err := NewPersistentStore(dataDir, AgentProfiles(AgentOptionsConfig{EnableMockAgent: true}))
	if err != nil {
		t.Fatalf("重新加载 Store 失败: %v", err)
	}
	snapshot := loaded.Snapshot()
	restored := findChatByID(snapshot.Chats, chat.ID)
	if restored == nil {
		t.Fatalf("恢复后缺少聊天页: %#v", snapshot.Chats)
	}
	if restored.Status != ChatStatusIdle || restored.AgentPersistence == nil || restored.AgentPersistence.SessionID != "thread-recoverable" {
		t.Fatalf("恢复后的聊天页状态或句柄不正确: %#v", restored)
	}
	timeline, err := loaded.FetchChatTimeline(chat.ID, ChatTimelineDirectionTail, nil, 200)
	if err != nil {
		t.Fatalf("读取恢复 timeline 失败: %v", err)
	}
	if !timelineRowsContainMessageStatus(timeline.Rows, MessageStatusStopped) {
		t.Fatalf("恢复 timeline 未包含 stopped 终态: %#v", timeline.Rows)
	}
}

// TestPersistentStoreIgnoresLegacyAgentSessionID 验证旧 session 字段不会补齐恢复句柄。
func TestPersistentStoreIgnoresLegacyAgentSessionID(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Now()
	project := Project{ID: "project-legacy", Name: "legacy", Path: t.TempDir(), CreatedAt: now, UpdatedAt: now}
	state := map[string]any{
		"schemaVersion": persistedStoreSchemaVersion,
		"projects":      []Project{project},
		"chats": []map[string]any{{
			"id":             "chat-legacy",
			"projectId":      project.ID,
			"title":          "旧聊天",
			"status":         ChatStatusIdle,
			"agentProvider":  AgentProviderMockCodex,
			"agentModel":     "mock-codex-gpt-5.5",
			"agentReasoning": "xhigh",
			"agentLocked":    true,
			"agentSessionId": "legacy-thread",
			"createdAt":      now.Format(time.RFC3339Nano),
			"updatedAt":      now.Format(time.RFC3339Nano),
		}},
		"agentProfiles":      AgentProfiles(AgentOptionsConfig{EnableMockAgent: true}),
		"lastAgentSelection": defaultLastAgentSelection(AgentProviderOptionsFromProfiles(AgentProfiles(AgentOptionsConfig{EnableMockAgent: true}))),
		"nextChatOrdinal":    map[string]int{project.ID: 1},
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("编码旧状态失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, persistedStateFileName), append(data, '\n'), 0600); err != nil {
		t.Fatalf("写入旧状态失败: %v", err)
	}
	loaded, err := NewPersistentStore(dataDir, AgentProfiles(AgentOptionsConfig{EnableMockAgent: true}))
	if err != nil {
		t.Fatalf("加载旧状态失败: %v", err)
	}
	restored := findChatByID(loaded.Snapshot().Chats, "chat-legacy")
	if restored == nil || restored.AgentPersistence != nil {
		t.Fatalf("旧 session 字段不应补齐恢复句柄: %#v", restored)
	}
}

// TestPersistentStoreDeltaDefersTimelineOnlySave 验证流式 delta 只延迟写对应 timeline，不重写 state.json。
func TestPersistentStoreDeltaDefersTimelineOnlySave(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewPersistentStore(dataDir, AgentProfiles(AgentOptionsConfig{}))
	if err != nil {
		t.Fatalf("创建持久化 Store 失败: %v", err)
	}
	project, err := store.CreateProject(t.TempDir())
	if err != nil {
		t.Fatalf("创建 project 失败: %v", err)
	}
	chat, err := store.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("创建聊天页失败: %v", err)
	}
	_, _, assistant, _, err := store.AddRunMessages(chat.ID, "性能热路径", nil, false)
	if err != nil {
		t.Fatalf("追加运行消息失败: %v", err)
	}
	statePath := filepath.Join(dataDir, persistedStateFileName)
	stateStat := mustStat(t, statePath)
	time.Sleep(20 * time.Millisecond)

	if _, _, ok := store.AppendAssistantDelta(chat.ID, assistant.ID, strings.Repeat("delta ", 4)); !ok {
		t.Fatalf("追加 assistant delta 失败")
	}
	if err := store.Flush(context.Background()); err != nil {
		t.Fatalf("刷新延迟 timeline 失败: %v", err)
	}
	if nextStat := mustStat(t, statePath); !nextStat.ModTime().Equal(stateStat.ModTime()) {
		t.Fatalf("流式 delta 不应重写 state.json: before=%s after=%s", stateStat.ModTime(), nextStat.ModTime())
	}
	data := readTimelineFile(t, dataDir, chat.ID)
	if !strings.Contains(string(data), "delta delta") {
		t.Fatalf("延迟刷新后 timeline 缺少 delta: %s", data)
	}
}

// TestJSONStorePersisterSaveChangesWritesOnlyDirtyTimeline 验证增量保存只写 dirty timeline。
func TestJSONStorePersisterSaveChangesWritesOnlyDirtyTimeline(t *testing.T) {
	dataDir := t.TempDir()
	persister := NewJSONStorePersister(dataDir)
	now := time.Now()
	project := Project{ID: "project-test", Name: "project", Path: t.TempDir(), CreatedAt: now, UpdatedAt: now}
	chatA := Chat{ID: "chat-a", ProjectID: project.ID, Title: "聊天 A", Status: ChatStatusIdle, AgentProvider: AgentProviderCodex, AgentModel: "gpt-5.5", CreatedAt: now, UpdatedAt: now}
	chatB := Chat{ID: "chat-b", ProjectID: project.ID, Title: "聊天 B", Status: ChatStatusIdle, AgentProvider: AgentProviderCodex, AgentModel: "gpt-5.5", CreatedAt: now, UpdatedAt: now}
	timelineA := timelineWithUserText(chatA.ID, "A", now)
	timelineB := timelineWithUserText(chatB.ID, "B", now)
	state := PersistedStoreState{
		Projects:           []Project{project},
		Chats:              []Chat{chatA, chatB},
		AgentProfiles:      AgentProfiles(AgentOptionsConfig{}),
		LastAgentSelection: defaultLastAgentSelection(DefaultAgentProviderOptions()),
		NextChatOrdinal:    map[string]int{project.ID: 2},
		ChatTimelines:      []PersistedChatTimeline{persistedChatTimelineFromState(timelineA), persistedChatTimelineFromState(timelineB)},
	}
	if err := persister.SaveAll(state); err != nil {
		t.Fatalf("完整保存失败: %v", err)
	}
	stateStat := mustStat(t, filepath.Join(dataDir, persistedStateFileName))
	chatBStat := mustStat(t, filepath.Join(dataDir, persistedTimelinesDirName, "chat-b.json"))
	time.Sleep(20 * time.Millisecond)

	_ = appendChatTimelineRow(&timelineA, ChatTimelineItem{Type: ChatTimelineItemAssistantDelta, MessageID: "msg-a", Delta: " updated"}, now)
	state.ChatTimelines = []PersistedChatTimeline{persistedChatTimelineFromState(timelineA), persistedChatTimelineFromState(timelineB)}
	if err := persister.SaveChanges(PersistedStoreChange{
		State:          state,
		DirtyChatIDs:   []string{chatA.ID},
		DeferTimelines: true,
	}); err != nil {
		t.Fatalf("增量保存失败: %v", err)
	}
	if err := persister.Flush(context.Background()); err != nil {
		t.Fatalf("刷新延迟 timeline 失败: %v", err)
	}
	if nextStateStat := mustStat(t, filepath.Join(dataDir, persistedStateFileName)); !nextStateStat.ModTime().Equal(stateStat.ModTime()) {
		t.Fatalf("timeline 变更不应重写 state.json")
	}
	if nextChatBStat := mustStat(t, filepath.Join(dataDir, persistedTimelinesDirName, "chat-b.json")); !nextChatBStat.ModTime().Equal(chatBStat.ModTime()) {
		t.Fatalf("未变更 timeline 不应被重写")
	}
	data := readTimelineFile(t, dataDir, chatA.ID)
	if !strings.Contains(string(data), "updated") {
		t.Fatalf("dirty timeline 未写入最新内容: %s", data)
	}
}

// TestPersistentStoreRejectsOldSchema 验证旧 schema 不再兼容加载。
func TestPersistentStoreRejectsOldSchema(t *testing.T) {
	dataDir := t.TempDir()
	state := PersistedStoreState{SchemaVersion: persistedStoreSchemaVersion - 1}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("编码旧状态失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, persistedStateFileName), append(data, '\n'), 0600); err != nil {
		t.Fatalf("写入旧状态失败: %v", err)
	}
	if _, err := NewPersistentStore(dataDir, AgentProfiles(AgentOptionsConfig{})); err == nil || !strings.Contains(err.Error(), "清理 AGENTHUB_DATA") {
		t.Fatalf("旧 schema 应提示清理数据目录: %v", err)
	}
}

// TestPersistentStoreSaveFailureKeepsMemory 验证保存失败时不会更新内存状态。
func TestPersistentStoreSaveFailureKeepsMemory(t *testing.T) {
	store := newStoreFromState(storeState{
		projects:        make(map[string]Project),
		chats:           make(map[string]Chat),
		timelines:       make(map[string]chatTimelineState),
		nextChatOrdinal: make(map[string]int),
		agentProfiles:   AgentProfiles(AgentOptionsConfig{}),
		lastAgent:       defaultLastAgentSelection(DefaultAgentProviderOptions()),
	}, failingStorePersister{})

	if _, err := store.CreateProject(t.TempDir()); err == nil {
		t.Fatalf("保存失败时创建 project 应返回错误")
	}
	snapshot := store.Snapshot()
	if len(snapshot.Projects) != 0 || len(snapshot.Chats) != 0 {
		t.Fatalf("保存失败后内存状态不应变化: %#v", snapshot)
	}
}

// TestPersistentStoreInvalidJSON 验证非法 JSON 会阻止启动且不覆盖原文件。
func TestPersistentStoreInvalidJSON(t *testing.T) {
	dataDir := t.TempDir()
	statePath := filepath.Join(dataDir, persistedStateFileName)
	if err := os.WriteFile(statePath, []byte("{invalid json"), 0600); err != nil {
		t.Fatalf("写入非法 JSON 失败: %v", err)
	}
	if _, err := NewPersistentStore(dataDir, AgentProfiles(AgentOptionsConfig{})); err == nil {
		t.Fatalf("非法 JSON 应导致加载失败")
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("读取非法 JSON 失败: %v", err)
	}
	if string(data) != "{invalid json" {
		t.Fatalf("非法 JSON 不应被覆盖: %s", data)
	}
}

// mustStat 使用 t 和 path 参数读取文件状态。
func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取文件状态失败 %s: %v", path, err)
	}
	return stat
}

// assertPersistentTimeline 使用 dataDir、chatID 和 text 参数断言 timeline 已持久化。
func assertPersistentTimeline(t *testing.T, dataDir string, chatID string, text string) {
	t.Helper()
	stateData, err := os.ReadFile(filepath.Join(dataDir, persistedStateFileName))
	if err != nil {
		t.Fatalf("读取 state.json 失败: %v", err)
	}
	var state PersistedStoreState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("解析 state.json 失败: %v", err)
	}
	if state.SchemaVersion != persistedStoreSchemaVersion {
		t.Fatalf("state.json schema 版本不正确: %d", state.SchemaVersion)
	}
	if strings.Contains(string(stateData), `"messages"`) || strings.Contains(string(stateData), `"usage"`) {
		t.Fatalf("state.json 不应包含聊天正文或用量: %s", stateData)
	}
	for _, chat := range state.Chats {
		if chat.ID == chatID && (len(chat.Messages) != 0 || chat.Plan != nil) {
			t.Fatalf("state.json 不应包含聊天正文: %#v", chat)
		}
	}
	data := readTimelineFile(t, dataDir, chatID)
	var timeline PersistedChatTimeline
	if err := json.Unmarshal(data, &timeline); err != nil {
		t.Fatalf("解析 timeline 文件失败: %v", err)
	}
	if timeline.SchemaVersion != persistedChatTimelineSchemaVersion {
		t.Fatalf("timeline schema 版本不正确: %d", timeline.SchemaVersion)
	}
	if !strings.Contains(string(data), text) {
		t.Fatalf("timeline 文件缺少目标文本: %s", data)
	}
}

// readTimelineFile 使用 t、dataDir 和 chatID 参数读取 timeline 文件。
func readTimelineFile(t *testing.T, dataDir string, chatID string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dataDir, persistedTimelinesDirName, chatID+".json"))
	if err != nil {
		t.Fatalf("读取 timeline 文件失败: %v", err)
	}
	return data
}

// timelineRowsContainMessageStatus 使用 rows 和 status 参数判断 timeline 是否包含指定消息终态。
func timelineRowsContainMessageStatus(rows []ChatTimelineRow, status string) bool {
	for _, row := range rows {
		if row.Item.Type == ChatTimelineItemMessageFinished && row.Item.Status == status {
			return true
		}
	}
	return false
}

// timelineWithUserText 使用 chatID、text 和 now 参数构造测试 timeline。
func timelineWithUserText(chatID string, text string, now time.Time) chatTimelineState {
	timeline := newChatTimelineState(chatID)
	messageID := "msg-" + strings.TrimPrefix(chatID, "chat-")
	_ = appendChatTimelineRow(&timeline, ChatTimelineItem{
		Type:      ChatTimelineItemMessageStarted,
		MessageID: messageID,
		Role:      MessageRoleUser,
		Text:      text,
		Status:    MessageStatusComplete,
	}, now)
	return timeline
}

// findChatByID 使用 chats 和 chatID 参数查找聊天页。
func findChatByID(chats []Chat, chatID string) *Chat {
	for index := range chats {
		if chats[index].ID == chatID {
			return &chats[index]
		}
	}
	return nil
}
