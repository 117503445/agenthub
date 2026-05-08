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

// chatTimelineFetchPayload 表示聊天 timeline 拉取请求。
type chatTimelineFetchPayload struct {
	ChatID    string                     `json:"chatId"`           // ChatID 表示聊天页标识。
	Direction string                     `json:"direction"`        // Direction 表示拉取方向。
	Cursor    *chatTimelineCursorPayload `json:"cursor,omitempty"` // Cursor 表示可选游标。
	Limit     int                        `json:"limit,omitempty"`  // Limit 表示最多返回行数。
}

// chatTimelineCursorPayload 表示 E2E 使用的 timeline 游标。
type chatTimelineCursorPayload struct {
	Epoch string `json:"epoch"` // Epoch 表示聊天 timeline 标识。
	Seq   int64  `json:"seq"`   // Seq 表示已知行序号。
}

// chatTimelineFetchResponse 表示聊天 timeline 拉取响应。
type chatTimelineFetchResponse struct {
	ChatID      string                     `json:"chatId"`                // ChatID 表示聊天页标识。
	Direction   string                     `json:"direction"`             // Direction 表示拉取方向。
	Epoch       string                     `json:"epoch"`                 // Epoch 表示聊天 timeline 标识。
	Reset       bool                       `json:"reset"`                 // Reset 表示需要重置本地状态。
	StaleCursor bool                       `json:"staleCursor"`           // StaleCursor 表示游标 epoch 已过期。
	Gap         bool                       `json:"gap"`                   // Gap 表示游标窗口缺口。
	Window      chatTimelineWindowPayload  `json:"window"`                // Window 表示服务端窗口。
	StartCursor *chatTimelineCursorPayload `json:"startCursor,omitempty"` // StartCursor 表示返回窗口起点。
	EndCursor   *chatTimelineCursorPayload `json:"endCursor,omitempty"`   // EndCursor 表示返回窗口终点。
	HasOlder    bool                       `json:"hasOlder"`              // HasOlder 表示还有更早行。
	HasNewer    bool                       `json:"hasNewer"`              // HasNewer 表示还有更新行。
	Rows        []chatTimelineRowPayload   `json:"rows"`                  // Rows 表示返回的 timeline 行。
}

// chatTimelineWindowPayload 表示 E2E 使用的 timeline 窗口。
type chatTimelineWindowPayload struct {
	MinSeq  int64 `json:"minSeq"`  // MinSeq 表示当前最小序号。
	MaxSeq  int64 `json:"maxSeq"`  // MaxSeq 表示当前最大序号。
	NextSeq int64 `json:"nextSeq"` // NextSeq 表示下一行序号。
}

// chatTimelineRowPayload 表示 E2E 使用的 timeline 行。
type chatTimelineRowPayload struct {
	ChatID    string          `json:"chatId"`    // ChatID 表示聊天页标识。
	Epoch     string          `json:"epoch"`     // Epoch 表示聊天 timeline 标识。
	Seq       int64           `json:"seq"`       // Seq 表示行序号。
	ID        string          `json:"id"`        // ID 表示行标识。
	Timestamp string          `json:"timestamp"` // Timestamp 表示行时间。
	Item      json.RawMessage `json:"item"`      // Item 表示 canonical 事件。
}

// chatTimelineAppendedPayload 表示 live timeline 追加事件。
type chatTimelineAppendedPayload struct {
	ChatID string                 `json:"chatId"` // ChatID 表示聊天页标识。
	Epoch  string                 `json:"epoch"`  // Epoch 表示聊天 timeline 标识。
	Row    chatTimelineRowPayload `json:"row"`    // Row 表示新增行。
}

// runChatTimelineAppendOnlyCase 使用 ctx 参数验证 per-chat append-only timeline。
func runChatTimelineAppendOnlyCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "Chat append-only timeline E2E 测试报告", success, events)
	}()

	fail := func(err error) bool {
		ctx.Logger.Errorf("Chat append-only timeline E2E 失败: %v", err)
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
	if len(snapshot.Chats[0].Messages) != 0 || snapshot.Chats[0].Plan != nil {
		return fail(fmt.Errorf("state.snapshot 不应包含聊天内容: %#v", snapshot.Chats[0]))
	}
	events = append(events, reportStep("state.snapshot 只包含聊天摘要，正文需要通过 timeline 拉取。"))

	initial, err := requestChatTimeline(conn, chatTimelineFetchPayload{ChatID: chatID, Direction: "tail", Limit: 200})
	if err != nil {
		return fail(err)
	}
	if initial.ChatID != chatID || initial.Epoch == "" || initial.Window.NextSeq <= 0 || len(initial.Rows) != 0 {
		return fail(fmt.Errorf("空聊天 timeline 初始窗口不正确: %#v", initial))
	}
	events = append(events, reportStep("空聊天可拉取 tail 窗口并初始化 per-chat epoch/seq。"))

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
		Prompt:   "append-only timeline 验证",
		PlanMode: false,
	}); err != nil {
		return fail(err)
	}
	firstRow, err := waitChatTimelineRow(conn, chatID, 10*time.Second)
	if err != nil {
		return fail(err)
	}
	if firstRow.Seq != initial.Window.NextSeq {
		return fail(fmt.Errorf("live row 序号未从 nextSeq 开始: first=%d next=%d", firstRow.Seq, initial.Window.NextSeq))
	}
	if _, err := readPersistenceMessage(conn, "agent.status", 20*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("发送消息后服务端通过 chat.timeline.appended 推送单调递增的 canonical row。"))

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
	if after.Rows[0].Seq != firstRow.Seq {
		return fail(fmt.Errorf("after 补齐起始行不正确: first=%d rows=%#v", firstRow.Seq, after.Rows))
	}
	events = append(events, reportStep("after 可按游标补齐断线期间新增 timeline 行。"))

	before, err := requestChatTimeline(conn, chatTimelineFetchPayload{
		ChatID:    chatID,
		Direction: "before",
		Cursor:    after.StartCursor,
		Limit:     1,
	})
	if err != nil {
		return fail(err)
	}
	if before.Reset || before.Gap || before.StaleCursor {
		return fail(fmt.Errorf("before 拉取不应触发 reset: %#v", before))
	}
	events = append(events, reportStep("before 可按游标拉取更早窗口，支持长聊天分页。"))

	reset, err := requestChatTimeline(conn, chatTimelineFetchPayload{
		ChatID:    chatID,
		Direction: "after",
		Cursor:    &chatTimelineCursorPayload{Epoch: "stale-epoch", Seq: 0},
		Limit:     200,
	})
	if err != nil {
		return fail(err)
	}
	if !reset.Reset || !reset.StaleCursor || reset.Epoch == "stale-epoch" {
		return fail(fmt.Errorf("过期 epoch 未返回 reset: %#v", reset))
	}
	events = append(events, reportStep("epoch 不匹配时，聊天 timeline 返回 canonical reset 窗口。"))

	conn.CloseNow()
	ctx.StopServer()
	restartStop, err := restartServerAtSameBaseURL(ctx, "chat-timeline-restart-logs")
	if err != nil {
		return fail(err)
	}
	defer restartStop()
	if err := waitUntilReady(ctx.BaseURL); err != nil {
		return fail(fmt.Errorf("重启服务未就绪: %w", err))
	}
	reconnectConn, err := dialPersistenceWS(ctx.BaseURL)
	if err != nil {
		return fail(err)
	}
	defer reconnectConn.CloseNow()
	if _, err := readPersistenceMessage(reconnectConn, "state.snapshot", 10*time.Second); err != nil {
		return fail(err)
	}
	restored, err := requestChatTimeline(reconnectConn, chatTimelineFetchPayload{ChatID: chatID, Direction: "tail", Limit: 200})
	if err != nil {
		return fail(err)
	}
	if restored.Epoch != after.Epoch || len(restored.Rows) == 0 || !chatTimelineRowsContainText(restored.Rows, "append-only timeline 验证") {
		return fail(fmt.Errorf("重启后 timeline 未恢复: epoch before=%s restored=%#v", after.Epoch, restored))
	}
	if err := assertChatTimelineFile(ctx.DataDir, chatID); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("后端重启后从 timelines/<chatID>.json 恢复 canonical rows。"))
	return true
}

// requestChatTimeline 使用 conn 和 payload 参数拉取聊天 timeline。
func requestChatTimeline(conn *websocket.Conn, payload chatTimelineFetchPayload) (chatTimelineFetchResponse, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return chatTimelineFetchResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, conn, persistenceClientMessage{Type: "chat.timeline.fetch", Payload: data}); err != nil {
		return chatTimelineFetchResponse{}, err
	}
	message, err := readPersistenceMessage(conn, "chat.timeline", 5*time.Second)
	if err != nil {
		return chatTimelineFetchResponse{}, err
	}
	var response chatTimelineFetchResponse
	if err := json.Unmarshal(message.Payload, &response); err != nil {
		return chatTimelineFetchResponse{}, err
	}
	return response, nil
}

// waitChatTimelineRow 使用 conn、chatID 和 timeout 参数等待 live timeline 行。
func waitChatTimelineRow(conn *websocket.Conn, chatID string, timeout time.Duration) (chatTimelineRowPayload, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		message, err := readPersistenceMessage(conn, "chat.timeline.appended", time.Until(deadline))
		if err != nil {
			return chatTimelineRowPayload{}, err
		}
		var payload chatTimelineAppendedPayload
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			return chatTimelineRowPayload{}, err
		}
		if payload.ChatID == chatID {
			return payload.Row, nil
		}
	}
	return chatTimelineRowPayload{}, fmt.Errorf("等待 chat.timeline.appended 超时: %s", chatID)
}

// chatTimelineRowsContainText 使用 rows 和 text 参数判断 timeline 是否包含目标文本。
func chatTimelineRowsContainText(rows []chatTimelineRowPayload, text string) bool {
	for _, row := range rows {
		if string(row.Item) != "" && json.Valid(row.Item) && strings.Contains(string(row.Item), text) {
			return true
		}
	}
	return false
}

// assertChatTimelineFile 使用 dataDir 和 chatID 参数断言 timeline 文件已写入。
func assertChatTimelineFile(dataDir string, chatID string) error {
	path := filepath.Join(dataDir, "timelines", chatID+".json")
	data, err := readFileForTimelineE2E(path)
	if err != nil {
		return err
	}
	var timeline struct {
		SchemaVersion int `json:"schemaVersion"` // SchemaVersion 表示 timeline 文件版本。
	}
	if err := json.Unmarshal(data, &timeline); err != nil {
		return err
	}
	if timeline.SchemaVersion != 4 {
		return fmt.Errorf("timeline 文件 schema 应为 v4: %d", timeline.SchemaVersion)
	}
	if !strings.Contains(string(data), chatID) {
		return fmt.Errorf("timeline 文件缺少 chatID: %s", path)
	}
	return nil
}

// readFileForTimelineE2E 使用 path 参数读取测试文件。
func readFileForTimelineE2E(path string) ([]byte, error) {
	return os.ReadFile(path)
}
