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

// chatTimelineSnapshotPayload 表示聊天 timeline E2E 使用的状态快照。
type chatTimelineSnapshotPayload struct {
	Projects []wsapp.Project `json:"projects"` // Projects 表示 project 列表。
	Chats    []wsapp.Chat    `json:"chats"`    // Chats 表示聊天页摘要列表。
}

// runChatTimelineFetchCase 使用 ctx 参数运行聊天 timeline 拉取 E2E 用例。
func runChatTimelineFetchCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "聊天 timeline 拉取 E2E 测试报告", success, events)
	}()

	fail := func(err error) bool {
		ctx.Logger.Errorf("聊天 timeline 拉取 E2E 失败: %v", err)
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
	snapshot, err := decodeChatTimelineSnapshot(snapshotMessage.Payload)
	if err != nil {
		return fail(err)
	}
	chat, err := firstChatTimelineChat(snapshot)
	if err != nil {
		return fail(err)
	}
	const promptTitle = "聊天 timeline 默认底部滚动测试"
	prompt := promptTitle + "\n" + strings.Repeat("历史消息应默认展示底部。\n", 120)
	if err := prepareChatTimelineHistory(conn, chat.ID, prompt); err != nil {
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
	reconnectSnapshot, err := decodeChatTimelineSnapshot(reconnectSnapshotMessage.Payload)
	if err != nil {
		return fail(err)
	}
	reconnectChat, err := findChatTimelineChat(reconnectSnapshot, chat.ID)
	if err != nil {
		return fail(err)
	}
	if len(reconnectChat.Messages) != 0 {
		return fail(fmt.Errorf("state.snapshot 不应包含聊天消息正文: chatID=%s messages=%d", reconnectChat.ID, len(reconnectChat.Messages)))
	}
	if reconnectChat.Plan != nil {
		return fail(fmt.Errorf("state.snapshot 不应包含聊天 plan: chatID=%s", reconnectChat.ID))
	}
	events = append(events, reportStep("重新连接后的 state.snapshot 只返回聊天页摘要，不携带历史消息和 plan。"))

	if err := assertChatTimelinePersistence(ctx.DataDir, chat.ID, promptTitle); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("AGENTHUB_DATA/state.json 只保存聊天摘要，聊天正文写入 timelines/<chatID>.json。"))

	timeline, err := requestChatTimeline(reconnectConn, chatTimelineFetchPayload{ChatID: chat.ID, Direction: "tail", Limit: 200})
	if err != nil {
		return fail(err)
	}
	if !chatTimelineRowsContainText(timeline.Rows, promptTitle) {
		return fail(fmt.Errorf("chat.timeline.fetch 未返回目标聊天历史: chatID=%s", chat.ID))
	}
	events = append(events, reportStep("前端请求 chat.timeline.fetch 后，后端返回 canonical rows 供本地 projector 生成消息。"))

	if err := assertChatTimelineFetchUI(ctx, snapshot.Projects[0].ID, chat.ID, promptTitle); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("浏览器直接进入聊天页 hash 路由后，前端拉取 timeline 并渲染历史消息；没有滚动记录时默认停在底部。"))
	return true
}

// prepareChatTimelineHistory 使用 conn、chatID 和 prompt 参数创建聊天历史。
func prepareChatTimelineHistory(conn *websocket.Conn, chatID string, prompt string) error {
	if err := sendE2EClientMessage(conn, "chat.agent.update", wsapp.ChatAgentUpdatePayload{
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
	if _, err := requestChatTimeline(conn, chatTimelineFetchPayload{ChatID: chatID, Direction: "tail", Limit: 200}); err != nil {
		return err
	}
	if err := sendE2EClientMessage(conn, "chat.send", wsapp.ChatSendPayload{
		ChatID:   chatID,
		Prompt:   prompt,
		PlanMode: false,
	}); err != nil {
		return err
	}
	return waitChatTimelineFinished(conn, chatID, 20*time.Second)
}

// sendE2EClientMessage 使用 conn、messageType 和 payload 参数发送 WebSocket 消息。
func sendE2EClientMessage(conn *websocket.Conn, messageType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return wsjson.Write(ctx, conn, persistenceClientMessage{Type: messageType, Payload: data})
}

// decodeChatTimelineSnapshot 使用 data 参数解析状态快照。
func decodeChatTimelineSnapshot(data json.RawMessage) (chatTimelineSnapshotPayload, error) {
	var payload chatTimelineSnapshotPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return chatTimelineSnapshotPayload{}, err
	}
	return payload, nil
}

// firstChatTimelineChat 使用 snapshot 参数返回第一个聊天页。
func firstChatTimelineChat(snapshot chatTimelineSnapshotPayload) (wsapp.Chat, error) {
	if len(snapshot.Projects) == 0 {
		return wsapp.Chat{}, fmt.Errorf("快照缺少 project")
	}
	if len(snapshot.Chats) == 0 {
		return wsapp.Chat{}, fmt.Errorf("快照缺少聊天页")
	}
	return snapshot.Chats[0], nil
}

// findChatTimelineChat 使用 snapshot 和 chatID 参数查找聊天页。
func findChatTimelineChat(snapshot chatTimelineSnapshotPayload, chatID string) (wsapp.Chat, error) {
	for _, chat := range snapshot.Chats {
		if chat.ID == chatID {
			return chat, nil
		}
	}
	return wsapp.Chat{}, fmt.Errorf("快照缺少目标聊天页: %s", chatID)
}

// waitChatTimelineFinished 使用 conn、chatID 和 timeout 参数等待当前聊天输出结束。
func waitChatTimelineFinished(conn *websocket.Conn, chatID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		message, err := readPersistenceMessage(conn, "chat.timeline.appended", time.Until(deadline))
		if err != nil {
			return err
		}
		var payload chatTimelineAppendedPayload
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			return err
		}
		if payload.ChatID != chatID {
			continue
		}
		var item wsapp.ChatTimelineItem
		if err := json.Unmarshal(payload.Row.Item, &item); err != nil {
			return err
		}
		if item.Type == wsapp.ChatTimelineItemMessageFinished {
			return nil
		}
	}
	return fmt.Errorf("等待聊天 timeline 完成超时: %s", chatID)
}

// assertChatTimelinePersistence 使用 dataDir、chatID 和 prompt 参数断言 timeline 持久化结果。
func assertChatTimelinePersistence(dataDir string, chatID string, prompt string) error {
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
	if strings.Contains(string(stateData), `"messages"`) || strings.Contains(string(stateData), `"usage"`) {
		return fmt.Errorf("state.json 不应保存聊天正文或 usage: chatID=%s", chatID)
	}
	for _, chat := range state.Chats {
		if chat.ID == chatID && len(chat.Messages) > 0 {
			return fmt.Errorf("state.json 不应保存聊天消息正文: chatID=%s messages=%d", chatID, len(chat.Messages))
		}
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "timelines", chatID+".json"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), prompt) {
		return fmt.Errorf("timeline 文件缺少目标消息: chatID=%s", chatID)
	}
	return nil
}

// assertChatTimelineFetchUI 使用 ctx、projectID、chatID 和 prompt 参数断言前端 timeline 拉取。
func assertChatTimelineFetchUI(ctx E2EContext, projectID string, chatID string, prompt string) error {
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
	if err := expectMessageLogScrollable(page, 48, 5*time.Second); err != nil {
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed-scrollable.png"), true)
		return err
	}
	if err := expectMessageLogScrolledToBottom(page, 8, 5*time.Second); err != nil {
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed-scroll-bottom.png"), true)
		return err
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-chat-timeline-fetch.png"), true)
	return nil
}
