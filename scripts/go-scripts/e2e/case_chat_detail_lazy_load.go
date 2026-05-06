package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/117503445/agenthub/internal/wsapp"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// chatDetailLazySnapshotPayload 表示聊天详情懒加载 E2E 使用的状态快照。
type chatDetailLazySnapshotPayload struct {
	Projects []wsapp.Project `json:"projects"` // Projects 表示 project 列表。
	Chats    []wsapp.Chat    `json:"chats"`    // Chats 表示聊天页摘要列表。
}

// chatDetailLazyDetailPayload 表示聊天详情懒加载 E2E 使用的详情响应。
type chatDetailLazyDetailPayload struct {
	Chat wsapp.Chat `json:"chat"` // Chat 表示完整聊天页详情。
}

// runChatDetailLazyLoadCase 使用 ctx 参数运行聊天详情懒加载 E2E 用例。
func runChatDetailLazyLoadCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeChatDetailLazyLoadReport(ctx.OutputDir, success, events)
	}()

	fail := func(err error) bool {
		ctx.Logger.Errorf("聊天详情懒加载 E2E 失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}

	conn, err := dialPersistenceWS(ctx.BaseURL)
	if err != nil {
		return fail(err)
	}
	defer conn.CloseNow()

	snapshotMessage, err := readPersistenceMessage(conn, "state.snapshot", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	snapshot, err := decodeChatDetailLazySnapshot(snapshotMessage.Payload)
	if err != nil {
		return fail(err)
	}
	chat, err := firstChatDetailLazyChat(snapshot)
	if err != nil {
		return fail(err)
	}
	const prompt = "聊天详情懒加载测试"
	if err := prepareChatDetailLazyHistory(conn, chat.ID, prompt); err != nil {
		return fail(err)
	}
	conn.CloseNow()
	events = append(events, reportStep("通过 WebSocket 在首个聊天页中写入一条历史消息。"))

	reconnectConn, err := dialPersistenceWS(ctx.BaseURL)
	if err != nil {
		return fail(err)
	}
	defer reconnectConn.CloseNow()
	reconnectSnapshotMessage, err := readPersistenceMessage(reconnectConn, "state.snapshot", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	reconnectSnapshot, err := decodeChatDetailLazySnapshot(reconnectSnapshotMessage.Payload)
	if err != nil {
		return fail(err)
	}
	reconnectChat, err := findChatDetailLazyChat(reconnectSnapshot, chat.ID)
	if err != nil {
		return fail(err)
	}
	if len(reconnectChat.Messages) != 0 {
		return fail(fmt.Errorf("state.snapshot 不应包含聊天消息详情: chatID=%s messages=%d", reconnectChat.ID, len(reconnectChat.Messages)))
	}
	if reconnectChat.Plan != nil {
		return fail(fmt.Errorf("state.snapshot 不应包含聊天 plan 详情: chatID=%s", reconnectChat.ID))
	}
	events = append(events, reportStep("重新连接后的 state.snapshot 只返回聊天页摘要，不携带历史消息和 plan 详情。"))

	if err := assertChatDetailLazyPersistence(ctx.DataDir, chat.ID, prompt); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("AGENTHUB_DATA/state.json 只保存聊天摘要，聊天详情写入 chats/<chatID>.json。"))

	if err := sendChatDetailLazyMessage(reconnectConn, "chat.detail.get", map[string]string{"chatId": chat.ID}); err != nil {
		return fail(err)
	}
	detailMessage, err := readPersistenceMessage(reconnectConn, "chat.detail", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	detail, err := decodeChatDetailLazyDetail(detailMessage.Payload)
	if err != nil {
		return fail(err)
	}
	if !chatDetailContainsPrompt(detail.Chat, prompt) {
		return fail(fmt.Errorf("chat.detail 未返回目标聊天历史: chatID=%s", chat.ID))
	}
	events = append(events, reportStep("前端请求 chat.detail.get 后，后端才返回该聊天页的完整消息详情。"))

	if err := assertChatDetailLazyLoadUI(ctx, snapshot.Projects[0].ID, chat.ID, prompt); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("浏览器直接进入聊天页 hash 路由后，前端请求详情并渲染历史消息。"))
	return true
}

// prepareChatDetailLazyHistory 使用 conn、chatID 和 prompt 参数创建聊天历史。
func prepareChatDetailLazyHistory(conn *websocket.Conn, chatID string, prompt string) error {
	if err := sendChatDetailLazyMessage(conn, "chat.agent.update", wsapp.ChatAgentUpdatePayload{
		ChatID:    chatID,
		Provider:  wsapp.AgentProviderMockCodex,
		Model:     "mock-codex-gpt-5.5",
		Reasoning: "xhigh",
	}); err != nil {
		return err
	}
	if _, err := readPersistenceMessage(conn, "chat.changed", 5*time.Second); err != nil {
		return err
	}
	if err := sendChatDetailLazyMessage(conn, "chat.detail.get", map[string]string{"chatId": chatID}); err != nil {
		return err
	}
	if _, err := readPersistenceMessage(conn, "chat.detail", 5*time.Second); err != nil {
		return err
	}
	if err := sendChatDetailLazyMessage(conn, "chat.send", wsapp.ChatSendPayload{
		ChatID:   chatID,
		Prompt:   prompt,
		PlanMode: false,
	}); err != nil {
		return err
	}
	if _, err := readPersistenceMessage(conn, "chat.message.done", 20*time.Second); err != nil {
		return err
	}
	return nil
}

// sendChatDetailLazyMessage 使用 conn、messageType 和 payload 参数发送 WebSocket 消息。
func sendChatDetailLazyMessage(conn *websocket.Conn, messageType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return wsjson.Write(ctx, conn, persistenceClientMessage{Type: messageType, Payload: data})
}

// decodeChatDetailLazySnapshot 使用 data 参数解析状态快照。
func decodeChatDetailLazySnapshot(data json.RawMessage) (chatDetailLazySnapshotPayload, error) {
	var payload chatDetailLazySnapshotPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return chatDetailLazySnapshotPayload{}, err
	}
	return payload, nil
}

// decodeChatDetailLazyDetail 使用 data 参数解析聊天详情响应。
func decodeChatDetailLazyDetail(data json.RawMessage) (chatDetailLazyDetailPayload, error) {
	var payload chatDetailLazyDetailPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return chatDetailLazyDetailPayload{}, err
	}
	return payload, nil
}

// firstChatDetailLazyChat 使用 snapshot 参数返回第一个聊天页。
func firstChatDetailLazyChat(snapshot chatDetailLazySnapshotPayload) (wsapp.Chat, error) {
	if len(snapshot.Projects) == 0 {
		return wsapp.Chat{}, fmt.Errorf("快照缺少 project")
	}
	if len(snapshot.Chats) == 0 {
		return wsapp.Chat{}, fmt.Errorf("快照缺少聊天页")
	}
	return snapshot.Chats[0], nil
}

// findChatDetailLazyChat 使用 snapshot 和 chatID 参数查找聊天页。
func findChatDetailLazyChat(snapshot chatDetailLazySnapshotPayload, chatID string) (wsapp.Chat, error) {
	for _, chat := range snapshot.Chats {
		if chat.ID == chatID {
			return chat, nil
		}
	}
	return wsapp.Chat{}, fmt.Errorf("快照缺少目标聊天页: %s", chatID)
}

// chatDetailContainsPrompt 使用 chat 和 prompt 参数判断详情中是否包含目标用户消息。
func chatDetailContainsPrompt(chat wsapp.Chat, prompt string) bool {
	for _, message := range chat.Messages {
		if message.Role == wsapp.MessageRoleUser && strings.Contains(message.Text, prompt) {
			return true
		}
	}
	return false
}

// assertChatDetailLazyPersistence 使用 dataDir、chatID 和 prompt 参数断言持久化拆分结果。
func assertChatDetailLazyPersistence(dataDir string, chatID string, prompt string) error {
	stateData, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	if err != nil {
		return err
	}
	var state struct {
		Chats []wsapp.Chat `json:"chats"` // Chats 表示 state.json 中的聊天摘要。
	}
	if err := json.Unmarshal(stateData, &state); err != nil {
		return err
	}
	for _, chat := range state.Chats {
		if chat.ID == chatID && len(chat.Messages) > 0 {
			return fmt.Errorf("state.json 不应保存聊天消息详情: chatID=%s messages=%d", chatID, len(chat.Messages))
		}
	}
	detailData, err := os.ReadFile(filepath.Join(dataDir, "chats", chatID+".json"))
	if err != nil {
		return err
	}
	var detail struct {
		Messages []wsapp.ChatMessage `json:"messages"` // Messages 表示聊天详情文件中的消息列表。
	}
	if err := json.Unmarshal(detailData, &detail); err != nil {
		return err
	}
	for _, message := range detail.Messages {
		if message.Role == wsapp.MessageRoleUser && strings.Contains(message.Text, prompt) {
			return nil
		}
	}
	return fmt.Errorf("聊天详情文件缺少目标消息: chatID=%s", chatID)
}

// assertChatDetailLazyLoadUI 使用 ctx、projectID、chatID 和 prompt 参数断言前端懒加载详情。
func assertChatDetailLazyLoadUI(ctx E2EContext, projectID string, chatID string, prompt string) error {
	session, err := newBrowserSession(1280, 820)
	if err != nil {
		return err
	}
	defer session.Close()
	page := session.page
	targetURL := ctx.BaseURL + "/#/projects/" + url.PathEscape(projectID) + "/chats/" + url.PathEscape(chatID)
	if err := gotoPage(page, targetURL); err != nil {
		return err
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return err
	}
	if err := expectTestIDText(page, "message-log", prompt, 10*time.Second); err != nil {
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed-ui.png"), true)
		return err
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-chat-detail-lazy-load.png"), true)
	return nil
}

// writeChatDetailLazyLoadReport 使用 outputDir、success 和 events 参数写入测试报告。
func writeChatDetailLazyLoadReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "聊天详情懒加载 E2E 测试报告", success, events)
}
