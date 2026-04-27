package wsapp

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestStoreProjectChatLifecycle 验证 project、聊天页和消息状态的完整生命周期。
func TestStoreProjectChatLifecycle(t *testing.T) {
	store := NewStore()
	projectPath := t.TempDir()

	project, err := store.CreateProject("  测试项目  ", projectPath)
	if err != nil {
		t.Fatalf("创建 project 失败: %v", err)
	}
	if project.Name != "测试项目" {
		t.Fatalf("project 名称未裁剪: %q", project.Name)
	}
	if project.Path != filepath.Clean(projectPath) {
		t.Fatalf("project 路径不正确: %q", project.Path)
	}

	chat, err := store.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("创建聊天页失败: %v", err)
	}
	if chat.Status != ChatStatusIdle || len(chat.Messages) != 0 {
		t.Fatalf("新聊天页状态不正确: status=%s messages=%d", chat.Status, len(chat.Messages))
	}

	chat, userMessage, assistantMessage, err := store.AddRunMessages(chat.ID, "  第一条 prompt  ")
	if err != nil {
		t.Fatalf("追加运行消息失败: %v", err)
	}
	if chat.Status != ChatStatusRunning {
		t.Fatalf("聊天页未进入运行状态: %s", chat.Status)
	}
	if userMessage.Text != "第一条 prompt" || userMessage.Status != MessageStatusComplete {
		t.Fatalf("用户消息不正确: %#v", userMessage)
	}
	if assistantMessage.Status != MessageStatusStreaming {
		t.Fatalf("assistant 占位消息不正确: %#v", assistantMessage)
	}

	if _, ok := store.AppendAssistantDelta(chat.ID, assistantMessage.ID, "Mock "); !ok {
		t.Fatal("追加第一段 assistant 增量失败")
	}
	if _, ok := store.AppendAssistantDelta(chat.ID, assistantMessage.ID, "Claude"); !ok {
		t.Fatal("追加第二段 assistant 增量失败")
	}
	chat, assistantMessage, ok := store.FinishAssistantMessage(chat.ID, assistantMessage.ID, MessageStatusComplete)
	if !ok {
		t.Fatal("结束 assistant 消息失败")
	}
	if chat.Status != ChatStatusIdle || assistantMessage.Text != "Mock Claude" {
		t.Fatalf("assistant 完成状态不正确: chat=%#v message=%#v", chat, assistantMessage)
	}

	if updatedChat, ok := store.SetChatSessionID(chat.ID, "session-1"); !ok || updatedChat.AgentSessionID != "session-1" {
		t.Fatalf("设置 session id 失败: ok=%v chat=%#v", ok, updatedChat)
	}

	chat, _, assistantMessage, err = store.AddRunMessages(chat.ID, "需要停止")
	if err != nil {
		t.Fatalf("追加待停止消息失败: %v", err)
	}
	chat, stoppedMessage, ok := store.StopStreamingMessage(chat.ID, MessageStatusStopped)
	if !ok {
		t.Fatal("停止流式消息失败")
	}
	if chat.Status != ChatStatusIdle || stoppedMessage.ID != assistantMessage.ID || stoppedMessage.Status != MessageStatusStopped {
		t.Fatalf("停止后的消息状态不正确: chat=%#v message=%#v", chat, stoppedMessage)
	}

	snapshot := store.Snapshot()
	if len(snapshot.Projects) != 1 || len(snapshot.Chats) != 1 {
		t.Fatalf("快照数量不正确: %#v", snapshot)
	}
	deletedChatIDs, err := store.DeleteProject(project.ID)
	if err != nil {
		t.Fatalf("删除 project 失败: %v", err)
	}
	if len(deletedChatIDs) != 1 || deletedChatIDs[0] != chat.ID {
		t.Fatalf("删除聊天页列表不正确: %#v", deletedChatIDs)
	}
	snapshot = store.Snapshot()
	if len(snapshot.Projects) != 0 || len(snapshot.Chats) != 0 {
		t.Fatalf("删除后的快照不为空: %#v", snapshot)
	}
}

// TestStoreRejectsInvalidProjectInput 验证 project 输入校验。
func TestStoreRejectsInvalidProjectInput(t *testing.T) {
	store := NewStore()

	if _, err := store.CreateProject("", t.TempDir()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("空名称应返回 ErrInvalidInput: %v", err)
	}
	if _, err := store.CreateProject("bad", filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("不存在的路径应返回 ErrInvalidInput: %v", err)
	}
	if _, err := store.CreateChat("missing-project"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在 project 创建聊天页应返回 ErrNotFound: %v", err)
	}
}
