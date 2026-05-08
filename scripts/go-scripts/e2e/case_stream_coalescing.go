package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/117503445/agenthub/internal/wsapp"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// runStreamCoalescingCase 使用 ctx 参数验证 Codex 高频 delta 会被合并后再推送前端。
func runStreamCoalescingCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "Codex 流式合并 E2E 测试报告", success, events)
	}()

	fail := func(err error) bool {
		ctx.Logger.Errorf("Codex 流式合并 E2E 失败: %v", err)
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
	if err := sendE2EClientMessage(conn, "chat.agent.update", wsapp.ChatAgentUpdatePayload{
		ChatID:    chat.ID,
		Provider:  wsapp.AgentProviderMockCodex,
		Model:     "mock-codex-gpt-5.5",
		Reasoning: "xhigh",
	}); err != nil {
		return fail(err)
	}
	if _, err := readPersistenceMessage(conn, "chat.changed", 5*time.Second); err != nil {
		return fail(err)
	}
	if _, err := requestChatTimeline(conn, chatTimelineFetchPayload{ChatID: chat.ID, Direction: "tail", Limit: 200}); err != nil {
		return fail(err)
	}

	if err := sendE2EClientMessage(conn, "chat.send", wsapp.ChatSendPayload{
		ChatID: chat.ID,
		Prompt: "MOCK_CODEX_DELTA_BURST " +
			strings.Repeat("delta ", 8),
	}); err != nil {
		return fail(err)
	}
	deltaCount, finalText, err := waitStreamCoalescingDone(conn, 20*time.Second)
	if err != nil {
		return fail(err)
	}
	if !strings.Contains(finalText, "Codex 流式合并输出完成") {
		return fail(fmt.Errorf("最终文本缺少流式合并标记: %q", finalText))
	}
	if deltaCount > 5 {
		return fail(fmt.Errorf("delta 推送未合并: got=%d want<=5", deltaCount))
	}

	events = append(events, reportStep(fmt.Sprintf("Mock Codex 高频输出被合并为 %d 次前端 delta。", deltaCount)))
	events = append(events, reportStep("最终 assistant 文本完整保留，未因合并丢失内容。"))
	return true
}

// waitStreamCoalescingDone 使用 conn 和 timeout 参数等待 assistant 输出完成并返回 delta 数量。
func waitStreamCoalescingDone(conn *websocket.Conn, timeout time.Duration) (int, string, error) {
	deadline := time.Now().Add(timeout)
	deltaCount := 0
	finalText := ""
	assistantMessageID := ""
	for time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(context.Background(), time.Until(deadline))
		var message persistenceServerMessage
		err := wsjson.Read(readCtx, conn, &message)
		cancel()
		if err != nil {
			return 0, "", err
		}
		if message.Type != "chat.timeline.appended" {
			continue
		}
		var payload chatTimelineAppendedPayload
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			return 0, "", err
		}
		var item wsapp.ChatTimelineItem
		if err := json.Unmarshal(payload.Row.Item, &item); err != nil {
			return 0, "", err
		}
		if item.Type == wsapp.ChatTimelineItemMessageStarted && item.Role == wsapp.MessageRoleAssistant {
			assistantMessageID = item.MessageID
		}
		if item.Type == wsapp.ChatTimelineItemAssistantDelta && (assistantMessageID == "" || item.MessageID == assistantMessageID) {
			deltaCount += 1
			finalText += item.Delta
			continue
		}
		if item.Type == wsapp.ChatTimelineItemMessageFinished && (assistantMessageID == "" || item.MessageID == assistantMessageID) && item.Status == wsapp.MessageStatusComplete {
			return deltaCount, finalText, nil
		}
	}
	return 0, "", fmt.Errorf("等待 assistant 完成超时")
}
