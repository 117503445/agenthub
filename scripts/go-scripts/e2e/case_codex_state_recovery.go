package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/117503445/agenthub/internal/wsapp"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// runCodexStateRecoveryCase 使用 ctx 参数验证 Codex 状态恢复对齐 Paseo。
func runCodexStateRecoveryCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "Codex 状态恢复 E2E 测试报告", success, events)
	}()

	fail := func(err error) bool {
		ctx.Logger.Errorf("Codex 状态恢复 E2E 失败: %v", err)
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
	if err := prepareCodexStateRecoveryChat(conn, chat.ID); err != nil {
		return fail(err)
	}
	if err := sendE2EClientMessage(conn, "chat.send", wsapp.ChatSendPayload{
		ChatID: chat.ID,
		Prompt: "MOCK_CODEX_RECOVERY_SLOW",
	}); err != nil {
		return fail(err)
	}
	if err := waitCodexStateRecoveryDelta(conn, chat.ID, 10*time.Second); err != nil {
		return fail(err)
	}
	sessionID, err := waitCodexResumeSessionID(ctx.DataDir, 10*time.Second)
	if err != nil {
		return fail(err)
	}
	if err := assertCodexStateRecoveryPersistence(ctx.DataDir, chat.ID, sessionID); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("运行中的 Codex 聊天已持久化 thread ID 和 provider persistence。"))

	ctx.StopServer()
	restartStop, err := restartServerAtSameBaseURL(ctx, "codex-state-recovery-restart-logs")
	if err != nil {
		return fail(err)
	}
	restartStopped := false
	defer func() {
		if !restartStopped {
			restartStop()
		}
	}()
	if err := waitUntilReady(ctx.BaseURL); err != nil {
		return fail(fmt.Errorf("重启服务未就绪: %w", err))
	}
	conn.CloseNow()
	restartConn, err := dialPersistenceWS(ctx.BaseURL)
	if err != nil {
		return fail(err)
	}
	defer restartConn.CloseNow()
	restartSnapshotMessage, err := readPersistenceMessage(restartConn, "state.snapshot", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	restartSnapshot, err := decodeChatTimelineSnapshot(restartSnapshotMessage.Payload)
	if err != nil {
		return fail(err)
	}
	restoredChat, err := findChatTimelineChat(restartSnapshot, chat.ID)
	if err != nil {
		return fail(err)
	}
	if restoredChat.Status != wsapp.ChatStatusIdle || restoredChat.AgentPersistence == nil || restoredChat.AgentPersistence.SessionID != sessionID {
		return fail(fmt.Errorf("重启后聊天状态或 thread ID 不正确: chat=%#v session=%s", restoredChat, sessionID))
	}
	restoredTimeline, err := requestChatTimeline(restartConn, chatTimelineFetchPayload{ChatID: chat.ID, Direction: "tail", Limit: 200})
	if err != nil {
		return fail(err)
	}
	if !chatTimelineRowsContainMessageStatus(restoredTimeline.Rows, wsapp.MessageStatusStopped) {
		return fail(fmt.Errorf("重启后未把 streaming assistant 标记为 stopped: %#v", restoredTimeline.Rows))
	}
	events = append(events, reportStep("后端重启后，运行中消息安全落为 stopped，聊天页恢复为可继续发送。"))

	if err := sendE2EClientMessage(restartConn, "chat.send", wsapp.ChatSendPayload{
		ChatID: chat.ID,
		Prompt: "MOCK_CODEX_RESUME_CONTEXT",
	}); err != nil {
		return fail(err)
	}
	if err := waitCodexStateRecoveryText(restartConn, chat.ID, "已恢复 Codex thread "+sessionID, 20*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("用户下一次发送时，AgentHub 使用原 Codex thread 继续对话。"))

	hydrateChatID, err := writeCodexStateRecoveryEmptyTimeline(ctx.DataDir, restoredChat, sessionID)
	if err != nil {
		return fail(err)
	}
	restartStop()
	restartStopped = true
	secondRestartStop, err := restartServerAtSameBaseURL(ctx, "codex-state-hydrate-restart-logs")
	if err != nil {
		return fail(err)
	}
	defer secondRestartStop()
	if err := waitUntilReady(ctx.BaseURL); err != nil {
		return fail(fmt.Errorf("第二次重启服务未就绪: %w", err))
	}
	hydrateConn, err := dialPersistenceWS(ctx.BaseURL)
	if err != nil {
		return fail(err)
	}
	defer hydrateConn.CloseNow()
	if _, err := readPersistenceMessage(hydrateConn, "state.snapshot", 5*time.Second); err != nil {
		return fail(err)
	}
	hydratedTimeline, err := requestChatTimeline(hydrateConn, chatTimelineFetchPayload{ChatID: hydrateChatID, Direction: "tail", Limit: 200})
	if err != nil {
		return fail(err)
	}
	if !chatTimelineRowsContainText(hydratedTimeline.Rows, "历史中恢复的 Codex 消息") {
		return fail(fmt.Errorf("空 timeline 未从 Codex 原生历史补齐: %#v", hydratedTimeline.Rows))
	}
	events = append(events, reportStep("空聊天 timeline 拉取 tail 时，会先从 Codex thread/read 懒加载原生历史。"))
	return true
}

// prepareCodexStateRecoveryChat 使用 conn 和 chatID 参数把聊天页切到 Mock Codex。
func prepareCodexStateRecoveryChat(conn *websocket.Conn, chatID string) error {
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
	_, err := requestChatTimeline(conn, chatTimelineFetchPayload{ChatID: chatID, Direction: "tail", Limit: 200})
	return err
}

// waitCodexStateRecoveryDelta 使用 conn、chatID 和 timeout 参数等待慢速恢复用例产生流式输出。
func waitCodexStateRecoveryDelta(conn *websocket.Conn, chatID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(context.Background(), time.Until(deadline))
		var message persistenceServerMessage
		err := wsjson.Read(readCtx, conn, &message)
		cancel()
		if err != nil {
			return err
		}
		if message.Type != "chat.timeline.appended" {
			continue
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
		if item.Type == wsapp.ChatTimelineItemAssistantDelta && strings.Contains(item.Delta, "Recovery") {
			return nil
		}
	}
	return fmt.Errorf("等待 Codex 慢速恢复 delta 超时")
}

// waitCodexStateRecoveryText 使用 conn、chatID、text 和 timeout 参数等待目标文本出现。
func waitCodexStateRecoveryText(conn *websocket.Conn, chatID string, text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(context.Background(), time.Until(deadline))
		var message persistenceServerMessage
		err := wsjson.Read(readCtx, conn, &message)
		cancel()
		if err != nil {
			return err
		}
		if message.Type != "chat.timeline.appended" {
			continue
		}
		var payload chatTimelineAppendedPayload
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			return err
		}
		if payload.ChatID == chatID && strings.Contains(string(payload.Row.Item), text) {
			return nil
		}
	}
	return fmt.Errorf("等待 Codex 恢复文本超时: %s", text)
}

// chatTimelineRowsContainMessageStatus 使用 rows 和 status 参数判断 timeline 是否包含目标消息终态。
func chatTimelineRowsContainMessageStatus(rows []chatTimelineRowPayload, status string) bool {
	for _, row := range rows {
		var item wsapp.ChatTimelineItem
		if err := json.Unmarshal(row.Item, &item); err != nil {
			continue
		}
		if item.Type == wsapp.ChatTimelineItemMessageFinished && item.Status == status {
			return true
		}
	}
	return false
}

// assertCodexStateRecoveryPersistence 使用 dataDir、chatID 和 sessionID 参数断言 provider persistence 已保存。
func assertCodexStateRecoveryPersistence(dataDir string, chatID string, sessionID string) error {
	data, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), `"agentPersistence"`) {
		return fmt.Errorf("state.json 缺少 agentPersistence")
	}
	if !strings.Contains(string(data), chatID) || !strings.Contains(string(data), sessionID) {
		return fmt.Errorf("state.json 缺少聊天或 session: chatID=%s sessionID=%s", chatID, sessionID)
	}
	return nil
}

// writeCodexStateRecoveryEmptyTimeline 使用 dataDir、source 和 sessionID 参数写入一个只有 persistence 的空聊天页。
func writeCodexStateRecoveryEmptyTimeline(dataDir string, source wsapp.Chat, sessionID string) (string, error) {
	statePath := filepath.Join(dataDir, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return "", err
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return "", err
	}
	chats, _ := state["chats"].([]any)
	chatID := "chat-e2e-empty-history"
	chat := map[string]any{
		"id":             chatID,
		"projectId":      source.ProjectID,
		"title":          "空历史恢复",
		"status":         wsapp.ChatStatusIdle,
		"agentProvider":  wsapp.AgentProviderMockCodex,
		"agentModel":     "mock-codex-gpt-5.5",
		"agentReasoning": "xhigh",
		"agentLocked":    true,
		"agentPersistence": map[string]any{
			"provider":     wsapp.AgentProviderMockCodex,
			"sessionId":    sessionID,
			"nativeHandle": sessionID,
			"metadata": map[string]any{
				"threadId": sessionID,
			},
		},
		"createdAt": time.Now().UTC().Format(time.RFC3339Nano),
		"updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	state["chats"] = append(chats, chat)
	nextData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(statePath, append(nextData, '\n'), 0600); err != nil {
		return "", err
	}
	timeline := map[string]any{
		"schemaVersion": 4,
		"chatId":        chatID,
		"epoch":         "epoch-e2e-empty-history",
		"nextSeq":       1,
		"rows":          []any{},
	}
	timelineData, err := json.MarshalIndent(timeline, "", "  ")
	if err != nil {
		return "", err
	}
	timelinePath := filepath.Join(dataDir, "timelines", chatID+".json")
	if err := os.MkdirAll(filepath.Dir(timelinePath), 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(timelinePath, append(timelineData, '\n'), 0600); err != nil {
		return "", err
	}
	return chatID, nil
}
