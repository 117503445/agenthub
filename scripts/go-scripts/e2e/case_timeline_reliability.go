package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/117503445/agenthub/internal/wsapp"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// timelineCatchUpPayload 表示 E2E 请求补齐 timeline 的参数。
type timelineCatchUpPayload struct {
	Epoch  string `json:"epoch"`  // Epoch 表示前端当前持有的 timeline 标识。
	EndSeq int64  `json:"endSeq"` // EndSeq 表示前端已应用到的最后序号。
}

// timelineCatchUpResponse 表示后端返回的 timeline 补齐结果。
type timelineCatchUpResponse struct {
	Epoch    string                     `json:"epoch"`              // Epoch 表示后端当前 timeline 标识。
	StartSeq int64                      `json:"startSeq"`           // StartSeq 表示补齐窗口起始序号。
	EndSeq   int64                      `json:"endSeq"`             // EndSeq 表示补齐窗口结束序号。
	Messages []persistenceServerMessage `json:"messages,omitempty"` // Messages 表示按序返回的历史消息。
	Reset    bool                       `json:"reset"`              // Reset 表示前端应重新拉取 canonical snapshot。
	Snapshot *wsapp.Snapshot            `json:"snapshot,omitempty"` // Snapshot 表示 reset 时返回的权威快照。
}

// runTimelineReliabilityCase 使用 ctx 参数验证 timeline 序号、catch-up 和 canonical snapshot。
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
	if snapshotMessage.Epoch == "" || snapshotMessage.Seq <= 0 {
		return fail(fmt.Errorf("state.snapshot 缺少 timeline 游标: epoch=%q seq=%d", snapshotMessage.Epoch, snapshotMessage.Seq))
	}
	var snapshot wsapp.Snapshot
	if err := json.Unmarshal(snapshotMessage.Payload, &snapshot); err != nil {
		return fail(err)
	}
	if len(snapshot.Chats) == 0 {
		return fail(fmt.Errorf("快照缺少聊天页"))
	}
	events = append(events, reportStep("state.snapshot 携带 epoch/seq，前端可用它初始化 timeline 游标。"))

	if err := sendChatDetailLazyMessage(conn, "chat.agent.update", wsapp.ChatAgentUpdatePayload{
		ChatID:    snapshot.Chats[0].ID,
		Provider:  wsapp.AgentProviderCodex,
		Model:     "gpt-5.5",
		Reasoning: "xhigh",
	}); err != nil {
		return fail(err)
	}
	changedMessage, err := readPersistenceMessage(conn, "chat.changed", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	if changedMessage.Seq <= snapshotMessage.Seq {
		return fail(fmt.Errorf("chat.changed 序号未递增: snapshot=%d changed=%d", snapshotMessage.Seq, changedMessage.Seq))
	}

	catchUp, err := requestTimelineCatchUp(conn, timelineCatchUpPayload{Epoch: snapshotMessage.Epoch, EndSeq: snapshotMessage.Seq})
	if err != nil {
		return fail(err)
	}
	if catchUp.Reset || catchUp.Epoch != snapshotMessage.Epoch || catchUp.EndSeq < changedMessage.Seq {
		return fail(fmt.Errorf("catch-up 未返回预期窗口: %#v changedSeq=%d", catchUp, changedMessage.Seq))
	}
	if !timelineCatchUpContains(catchUp.Messages, "chat.changed", changedMessage.Seq) {
		return fail(fmt.Errorf("catch-up 缺少已广播 chat.changed: %#v", catchUp.Messages))
	}
	events = append(events, reportStep("timeline.catch_up 可按 epoch/endSeq 补齐断线期间错过的事件，并保留原始消息顺序。"))

	resetCatchUp, err := requestTimelineCatchUp(conn, timelineCatchUpPayload{Epoch: "stale-epoch", EndSeq: 0})
	if err != nil {
		return fail(err)
	}
	if !resetCatchUp.Reset || resetCatchUp.Snapshot == nil {
		return fail(fmt.Errorf("过期 epoch 未返回 canonical snapshot: %#v", resetCatchUp))
	}
	events = append(events, reportStep("epoch 不匹配时，后端返回 canonical snapshot，前端可重置状态并避免 gap 放大。"))
	return true
}

// requestTimelineCatchUp 使用 conn 和 payload 参数请求 timeline 补齐。
func requestTimelineCatchUp(conn *websocket.Conn, payload timelineCatchUpPayload) (timelineCatchUpResponse, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return timelineCatchUpResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, conn, persistenceClientMessage{Type: "timeline.catch_up", Payload: data}); err != nil {
		return timelineCatchUpResponse{}, err
	}
	message, err := readPersistenceMessage(conn, "timeline.catch_up", 5*time.Second)
	if err != nil {
		return timelineCatchUpResponse{}, err
	}
	var response timelineCatchUpResponse
	if err := json.Unmarshal(message.Payload, &response); err != nil {
		return timelineCatchUpResponse{}, err
	}
	return response, nil
}

// timelineCatchUpContains 使用 messages、messageType 和 seq 参数判断补齐结果是否包含目标消息。
func timelineCatchUpContains(messages []persistenceServerMessage, messageType string, seq int64) bool {
	for _, message := range messages {
		if message.Type == messageType && message.Seq == seq {
			return true
		}
	}
	return false
}
