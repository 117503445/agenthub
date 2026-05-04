package wsapp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// failingStorePersister 表示始终保存失败的测试持久化器。
type failingStorePersister struct{}

// Save 使用 state 参数模拟保存失败。
func (f failingStorePersister) Save(state PersistedStoreState) error {
	return errors.New("forced save failure")
}

// TestPersistentStoreSaveLoad 验证 Store 状态可保存并从 JSON 恢复。
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
	if _, _, assistant, err := store.AddRunMessages(chat.ID, "持久化测试", nil, false); err != nil {
		t.Fatalf("追加运行消息失败: %v", err)
	} else {
		if _, ok := store.AppendAssistantDelta(chat.ID, assistant.ID, "完成"); !ok {
			t.Fatalf("追加 assistant 输出失败")
		}
		if _, _, ok := store.FinishAssistantMessage(chat.ID, assistant.ID, MessageStatusComplete); !ok {
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
	if _, err := os.Stat(filepath.Join(dataDir, persistedStateFileName)); err != nil {
		t.Fatalf("状态文件未写入: %v", err)
	}

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
	if snapshot.LastAgentSelection.Provider != AgentProviderCodex || snapshot.LastAgentSelection.Model != "gpt-5.5" {
		t.Fatalf("恢复后的上次 agent 选择不正确: %#v", snapshot.LastAgentSelection)
	}
	nextChat, err := loaded.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("恢复后创建聊天页失败: %v", err)
	}
	if nextChat.Title != "聊天 3" {
		t.Fatalf("恢复后的聊天页序号不正确: %q", nextChat.Title)
	}
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

// TestPersistentStoreSaveFailureKeepsMemory 验证保存失败时不会更新内存状态。
func TestPersistentStoreSaveFailureKeepsMemory(t *testing.T) {
	store := newStoreFromState(storeState{
		projects:        make(map[string]Project),
		chats:           make(map[string]Chat),
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

// TestPersistentStoreNormalizesRunningState 验证启动加载时会停止无法恢复的运行中状态。
func TestPersistentStoreNormalizesRunningState(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Now()
	project := Project{ID: "project-test", Name: "project", Path: t.TempDir(), CreatedAt: now, UpdatedAt: now}
	chat := Chat{
		ID:            "chat-test",
		ProjectID:     project.ID,
		Title:         "聊天 1",
		Status:        ChatStatusRunning,
		AgentProvider: AgentProviderMockClaudeCode,
		AgentModel:    "mock-claude-sonnet",
		Messages: []ChatMessage{{
			ID:        "msg-test",
			ChatID:    "chat-test",
			Role:      MessageRoleAssistant,
			Status:    MessageStatusStreaming,
			CreatedAt: now,
			UpdatedAt: now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	persister := NewJSONStorePersister(dataDir)
	if err := persister.Save(PersistedStoreState{
		Projects:           []Project{project},
		Chats:              []Chat{chat},
		AgentProfiles:      AgentProfiles(AgentOptionsConfig{EnableMockAgent: true}),
		LastAgentSelection: defaultLastAgentSelection(DefaultAgentProviderOptions()),
		NextChatOrdinal:    map[string]int{project.ID: 1},
	}); err != nil {
		t.Fatalf("写入测试状态失败: %v", err)
	}

	store, err := NewPersistentStore(dataDir, AgentProfiles(AgentOptionsConfig{EnableMockAgent: true}))
	if err != nil {
		t.Fatalf("加载持久化 Store 失败: %v", err)
	}
	snapshot := store.Snapshot()
	if len(snapshot.Chats) != 1 {
		t.Fatalf("恢复后的聊天页数量不正确: %#v", snapshot.Chats)
	}
	loadedChat := snapshot.Chats[0]
	if loadedChat.Status != ChatStatusIdle || loadedChat.Messages[0].Status != MessageStatusStopped {
		t.Fatalf("运行中状态未归一为停止: %#v", loadedChat)
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
