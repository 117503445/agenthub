package e2e

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/117503445/agenthub/internal/wsapp"
)

// runTimelineReliabilityCase 使用 ctx 参数验证 per-chat timeline 游标、补齐和 reset。
func runTimelineReliabilityCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "Timeline 可靠性 E2E 测试报告", success, events)
	}()

	fail := func(err error) bool {
		ctx.Logger.Errorf("Timeline 可靠性 E2E 失败: %v", err)
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
	var snapshot wsapp.Snapshot
	if err := json.Unmarshal(snapshotMessage.Payload, &snapshot); err != nil {
		return fail(err)
	}
	if len(snapshot.Chats) == 0 {
		return fail(fmt.Errorf("快照缺少聊天页"))
	}
	chatID := snapshot.Chats[0].ID
	initial, err := requestChatTimeline(conn, chatTimelineFetchPayload{ChatID: chatID, Direction: "tail", Limit: 200})
	if err != nil {
		return fail(err)
	}
	if initial.Epoch == "" || initial.Window.NextSeq <= 0 {
		return fail(fmt.Errorf("初始 timeline 游标不正确: %#v", initial))
	}
	events = append(events, reportStep("state.snapshot 不再携带全局游标，前端通过 chat.timeline.fetch 初始化聊天 timeline。"))

	if err := sendE2EClientMessage(conn, "chat.agent.update", wsapp.ChatAgentUpdatePayload{
		ChatID:    chatID,
		Provider:  wsapp.AgentProviderMockCodex,
		Model:     "mock-codex-gpt-5.5",
		Reasoning: "xhigh",
	}); err != nil {
		return fail(err)
	}
	if _, err := readPersistenceMessage(conn, "chat.changed", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := sendE2EClientMessage(conn, "chat.send", wsapp.ChatSendPayload{
		ChatID:   chatID,
		Prompt:   "per-chat timeline reliability 验证",
		PlanMode: false,
	}); err != nil {
		return fail(err)
	}
	firstRow, err := waitChatTimelineRow(conn, chatID, 10*time.Second)
	if err != nil {
		return fail(err)
	}
	if firstRow.Epoch != initial.Epoch || firstRow.Seq < initial.Window.NextSeq {
		return fail(fmt.Errorf("live timeline row 游标异常: initial=%#v row=%#v", initial, firstRow))
	}

	after, err := requestChatTimeline(conn, chatTimelineFetchPayload{
		ChatID:    chatID,
		Direction: "after",
		Cursor:    &chatTimelineCursorPayload{Epoch: initial.Epoch, Seq: initial.Window.NextSeq - 1},
		Limit:     0,
	})
	if err != nil {
		return fail(err)
	}
	if after.Reset || after.Gap || after.StaleCursor || len(after.Rows) == 0 {
		return fail(fmt.Errorf("after 补齐未返回新增行: %#v", after))
	}
	events = append(events, reportStep("chat.timeline.fetch after 可按 per-chat epoch/seq 补齐断线期间错过的行。"))

	before, err := requestChatTimeline(conn, chatTimelineFetchPayload{
		ChatID:    chatID,
		Direction: "before",
		Cursor:    after.EndCursor,
		Limit:     1,
	})
	if err != nil {
		return fail(err)
	}
	if before.Reset || before.Gap || before.StaleCursor || len(before.Rows) == 0 {
		return fail(fmt.Errorf("before 未返回预期窗口: %#v", before))
	}
	events = append(events, reportStep("chat.timeline.fetch before 可拉取更早窗口，支持长聊天分页。"))

	gap, err := requestChatTimeline(conn, chatTimelineFetchPayload{
		ChatID:    chatID,
		Direction: "after",
		Cursor:    &chatTimelineCursorPayload{Epoch: initial.Epoch, Seq: -10},
		Limit:     200,
	})
	if err != nil {
		return fail(err)
	}
	if !gap.Reset || !gap.Gap {
		return fail(fmt.Errorf("窗口缺口未返回 reset/gap: %#v", gap))
	}

	stale, err := requestChatTimeline(conn, chatTimelineFetchPayload{
		ChatID:    chatID,
		Direction: "after",
		Cursor:    &chatTimelineCursorPayload{Epoch: "stale-epoch", Seq: 0},
		Limit:     200,
	})
	if err != nil {
		return fail(err)
	}
	if !stale.Reset || !stale.StaleCursor || stale.Epoch != initial.Epoch {
		return fail(fmt.Errorf("过期 epoch 未返回 canonical reset: %#v", stale))
	}
	events = append(events, reportStep("gap 或 epoch 不匹配时，后端返回 canonical reset 窗口，前端可安全重建本地 timeline。"))
	return true
}
