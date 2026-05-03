package wsapp

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestStoreProjectChatLifecycle 验证 project、聊天页和消息状态的完整生命周期。
func TestStoreProjectChatLifecycle(t *testing.T) {
	store := NewStore()
	projectPath := t.TempDir()

	project, err := store.CreateProject(projectPath)
	if err != nil {
		t.Fatalf("创建 project 失败: %v", err)
	}
	if project.Name != filepath.Base(projectPath) {
		t.Fatalf("project 名称未从路径派生: %q", project.Name)
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
	if chat.AgentProvider != AgentProviderMockClaudeCode || chat.AgentModel != "mock-claude-sonnet" {
		t.Fatalf("新聊天页 agent 默认值不正确: %#v", chat)
	}
	chat, err = store.UpdateChatAgent(chat.ID, AgentProviderCodex, "gpt-5.5", "xhigh")
	if err != nil {
		t.Fatalf("更新聊天页 agent 失败: %v", err)
	}
	if chat.AgentProvider != AgentProviderCodex || chat.AgentModel != "gpt-5.5" || chat.AgentReasoning != "xhigh" {
		t.Fatalf("聊天页 agent 未更新: %#v", chat)
	}

	chat, userMessage, assistantMessage, err := store.AddRunMessages(chat.ID, "  第一条 prompt  ", nil, false)
	if err != nil {
		t.Fatalf("追加运行消息失败: %v", err)
	}
	if chat.Status != ChatStatusRunning {
		t.Fatalf("聊天页未进入运行状态: %s", chat.Status)
	}
	if chat.Title != "第一条 prompt" {
		t.Fatalf("聊天页标题未从首条 prompt 派生: %q", chat.Title)
	}
	if userMessage.Text != "第一条 prompt" || userMessage.Status != MessageStatusComplete {
		t.Fatalf("用户消息不正确: %#v", userMessage)
	}
	if assistantMessage.Status != MessageStatusStreaming {
		t.Fatalf("assistant 占位消息不正确: %#v", assistantMessage)
	}
	if !chat.AgentLocked {
		t.Fatalf("聊天页开始运行后应该锁定 agent: %#v", chat)
	}
	if _, err := store.UpdateChatAgent(chat.ID, AgentProviderClaudeCode, "sonnet", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("agent 锁定后不应允许切换 provider: %v", err)
	}
	chat, err = store.UpdateChatAgent(chat.ID, AgentProviderCodex, "gpt-5.4", "")
	if err != nil {
		t.Fatalf("agent 锁定后仍应允许更新模型: %v", err)
	}
	if chat.AgentModel != "gpt-5.4" {
		t.Fatalf("锁定后的模型未更新: %#v", chat)
	}

	if _, ok := store.AppendAssistantDelta(chat.ID, assistantMessage.ID, "Mock "); !ok {
		t.Fatal("追加第一段 assistant 增量失败")
	}
	chat, toolMessage, ok := store.UpsertToolCall(chat.ID, assistantMessage.ID, ToolCall{
		ID:     "tool-1",
		Name:   "Read",
		Status: ToolCallStatusRunning,
		Input:  `{"file_path":"README.md"}`,
	})
	if !ok {
		t.Fatal("插入工具调用失败")
	}
	if len(toolMessage.ToolCalls) != 1 || toolMessage.ToolCalls[0].Name != "Read" {
		t.Fatalf("工具调用插入不正确: %#v", toolMessage.ToolCalls)
	}
	chat, toolMessage, ok = store.UpsertToolCall(chat.ID, assistantMessage.ID, ToolCall{
		ID:     "tool-1",
		Status: ToolCallStatusComplete,
		Output: "完成",
	})
	if !ok {
		t.Fatal("更新工具调用失败")
	}
	if toolMessage.ToolCalls[0].Status != ToolCallStatusComplete || toolMessage.ToolCalls[0].Output != "完成" {
		t.Fatalf("工具调用更新不正确: %#v chat=%#v", toolMessage.ToolCalls, chat)
	}
	if _, ok := store.AppendAssistantDelta(chat.ID, assistantMessage.ID, "Claude"); !ok {
		t.Fatal("追加第二段 assistant 增量失败")
	}
	chat, assistantMessage, ok = store.FinishAssistantMessage(chat.ID, assistantMessage.ID, MessageStatusComplete)
	if !ok {
		t.Fatal("结束 assistant 消息失败")
	}
	if chat.Status != ChatStatusIdle || assistantMessage.Text != "Mock Claude" {
		t.Fatalf("assistant 完成状态不正确: chat=%#v message=%#v", chat, assistantMessage)
	}
	if len(assistantMessage.Parts) != 3 {
		t.Fatalf("assistant 事件分段数量不正确: %#v", assistantMessage.Parts)
	}
	if assistantMessage.Parts[0].Type != MessagePartTypeText || assistantMessage.Parts[1].Type != MessagePartTypeToolCall || assistantMessage.Parts[2].Type != MessagePartTypeText {
		t.Fatalf("assistant 事件分段顺序不正确: %#v", assistantMessage.Parts)
	}

	if updatedChat, ok := store.SetChatSessionID(chat.ID, "session-1"); !ok || updatedChat.AgentSessionID != "session-1" {
		t.Fatalf("设置 session id 失败: ok=%v chat=%#v", ok, updatedChat)
	}

	chat, _, assistantMessage, err = store.AddRunMessages(chat.ID, "需要停止", nil, false)
	if err != nil {
		t.Fatalf("追加待停止消息失败: %v", err)
	}
	if chat.Title != "第一条 prompt" {
		t.Fatalf("聊天页标题不应被后续 prompt 覆盖: %q", chat.Title)
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
	options, err := store.AddAgentModel(AgentProviderClaudeCode, "claude-custom-test", "Claude Custom Test")
	if err != nil {
		t.Fatalf("新增 Claude Code 模型失败: %v", err)
	}
	if !agentOptionsContainModel(options, AgentProviderClaudeCode, "claude-custom-test") {
		t.Fatalf("新增模型未出现在 agent 选项中: %#v", options)
	}
	extraChat, err := store.CreateChat(project.ID)
	if err != nil {
		t.Fatalf("创建待删除聊天页失败: %v", err)
	}
	if extraChat.AgentProvider != AgentProviderCodex || extraChat.AgentModel != "gpt-5.4" {
		t.Fatalf("新聊天页未继承上次 agent 选择: %#v", extraChat)
	}
	deletedProjectID, err := store.DeleteChat(extraChat.ID)
	if err != nil {
		t.Fatalf("删除单个聊天页失败: %v", err)
	}
	if deletedProjectID != project.ID {
		t.Fatalf("删除聊天页返回的 project 不正确: %q", deletedProjectID)
	}
	snapshot = store.Snapshot()
	if len(snapshot.Chats) != 1 {
		t.Fatalf("删除单个聊天页后数量不正确: %#v", snapshot.Chats)
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

// TestDeriveChatTitleFromPrompt 验证聊天页标题派生规则。
func TestDeriveChatTitleFromPrompt(t *testing.T) {
	title := deriveChatTitleFromPrompt(" \n\t  修复   登录页   样式  \n第二行")
	if title != "修复 登录页 样式" {
		t.Fatalf("标题未正确取首个非空行并压缩空白: %q", title)
	}

	longPrompt := strings.Repeat("测", maxChatTitleRunes+5)
	title = deriveChatTitleFromPrompt(longPrompt)
	if got := len([]rune(title)); got != maxChatTitleRunes {
		t.Fatalf("标题长度未截断到 %d 个字符: %d", maxChatTitleRunes, got)
	}

	if title := deriveChatTitleFromPrompt(" \n\t "); title != "" {
		t.Fatalf("空 prompt 不应生成标题: %q", title)
	}
}

// agentOptionsContainModel 使用 options、provider 和 model 参数判断模型是否存在。
func agentOptionsContainModel(options []AgentProviderOption, provider string, model string) bool {
	for _, option := range options {
		if option.ID != provider {
			continue
		}
		for _, item := range option.Models {
			if item.ID == model {
				return true
			}
		}
	}
	return false
}

// TestStoreRejectsInvalidProjectInput 验证 project 输入校验。
func TestStoreRejectsInvalidProjectInput(t *testing.T) {
	store := NewStore()

	if _, err := store.CreateProject(""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("空路径应返回 ErrInvalidInput: %v", err)
	}
	if _, err := store.CreateProject(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("不存在的路径应返回 ErrInvalidInput: %v", err)
	}
	if _, err := store.CreateChat("missing-project"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在 project 创建聊天页应返回 ErrNotFound: %v", err)
	}
}
