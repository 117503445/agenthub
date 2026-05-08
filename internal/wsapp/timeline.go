package wsapp

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// ChatTimelineDirectionTail 表示从末尾拉取 timeline。
	ChatTimelineDirectionTail = "tail"
	// ChatTimelineDirectionAfter 表示拉取游标之后的 timeline。
	ChatTimelineDirectionAfter = "after"
	// ChatTimelineDirectionBefore 表示拉取游标之前的 timeline。
	ChatTimelineDirectionBefore = "before"
)

const (
	// ChatTimelineItemMessageStarted 表示一条消息开始。
	ChatTimelineItemMessageStarted = "message_started"
	// ChatTimelineItemAssistantDelta 表示 assistant 文本增量。
	ChatTimelineItemAssistantDelta = "assistant_delta"
	// ChatTimelineItemToolCall 表示工具调用状态更新。
	ChatTimelineItemToolCall = "tool_call"
	// ChatTimelineItemUsageUpdated 表示用量信息更新。
	ChatTimelineItemUsageUpdated = "usage_updated"
	// ChatTimelineItemMessageFinished 表示消息结束。
	ChatTimelineItemMessageFinished = "message_finished"
	// ChatTimelineItemPlanSet 表示写入待确认 plan。
	ChatTimelineItemPlanSet = "plan_set"
	// ChatTimelineItemPlanStatusChanged 表示 plan 状态变化。
	ChatTimelineItemPlanStatusChanged = "plan_status_changed"
	// ChatTimelineItemSystemMessage 表示系统消息。
	ChatTimelineItemSystemMessage = "system_message"
)

const (
	// defaultChatTimelineFetchLimit 表示默认 timeline 拉取数量。
	defaultChatTimelineFetchLimit = 200
)

// ChatTimelineItem 表示聊天 canonical timeline 中的一条业务事件。
type ChatTimelineItem struct {
	Type      string         `json:"type"`                // Type 表示事件类型。
	MessageID string         `json:"messageId,omitempty"` // MessageID 表示关联消息标识。
	Role      string         `json:"role,omitempty"`      // Role 表示消息角色。
	Text      string         `json:"text,omitempty"`      // Text 表示消息文本或完整正文。
	Delta     string         `json:"delta,omitempty"`     // Delta 表示 assistant 文本增量。
	Status    string         `json:"status,omitempty"`    // Status 表示消息或 plan 状态。
	ToolCall  *ToolCall      `json:"toolCall,omitempty"`  // ToolCall 表示工具调用状态。
	Usage     *AgentUsage    `json:"usage,omitempty"`     // Usage 表示用量信息。
	Plan      *PlanApproval  `json:"plan,omitempty"`      // Plan 表示待确认 plan。
	Images    []MessageImage `json:"images,omitempty"`    // Images 表示用户消息图片。
}

// ChatTimelineRow 表示聊天 append-only timeline 中的一行。
type ChatTimelineRow struct {
	ID        string           `json:"id"`        // ID 表示行标识。
	ChatID    string           `json:"chatId"`    // ChatID 表示聊天页标识。
	Epoch     string           `json:"epoch"`     // Epoch 表示 timeline 世代标识。
	Seq       int64            `json:"seq"`       // Seq 表示单调递增序号。
	Timestamp time.Time        `json:"timestamp"` // Timestamp 表示写入时间。
	Item      ChatTimelineItem `json:"item"`      // Item 表示业务事件。
}

// ChatTimelineCursor 表示聊天 timeline 游标。
type ChatTimelineCursor struct {
	Epoch string `json:"epoch"` // Epoch 表示 timeline 世代标识。
	Seq   int64  `json:"seq"`   // Seq 表示行序号。
}

// ChatTimelineWindow 表示当前 timeline 窗口范围。
type ChatTimelineWindow struct {
	MinSeq  int64 `json:"minSeq"`  // MinSeq 表示当前最小序号。
	MaxSeq  int64 `json:"maxSeq"`  // MaxSeq 表示当前最大序号。
	NextSeq int64 `json:"nextSeq"` // NextSeq 表示下一行序号。
}

// ChatTimelineFetchResult 表示 timeline 拉取结果。
type ChatTimelineFetchResult struct {
	ChatID      string              `json:"chatId"`                // ChatID 表示聊天页标识。
	Direction   string              `json:"direction"`             // Direction 表示拉取方向。
	Epoch       string              `json:"epoch"`                 // Epoch 表示 timeline 世代标识。
	Reset       bool                `json:"reset"`                 // Reset 表示前端应重置本地窗口。
	StaleCursor bool                `json:"staleCursor"`           // StaleCursor 表示游标 epoch 过期。
	Gap         bool                `json:"gap"`                   // Gap 表示游标已落在窗口之外。
	Window      ChatTimelineWindow  `json:"window"`                // Window 表示服务端窗口。
	StartCursor *ChatTimelineCursor `json:"startCursor,omitempty"` // StartCursor 表示返回行起点。
	EndCursor   *ChatTimelineCursor `json:"endCursor,omitempty"`   // EndCursor 表示返回行终点。
	HasOlder    bool                `json:"hasOlder"`              // HasOlder 表示还有更早行。
	HasNewer    bool                `json:"hasNewer"`              // HasNewer 表示还有更新行。
	Rows        []ChatTimelineRow   `json:"rows"`                  // Rows 表示返回的行。
}

// chatTimelineState 表示 Store 内部维护的单个聊天 timeline。
type chatTimelineState struct {
	ChatID  string            `json:"chatId"`  // ChatID 表示聊天页标识。
	Epoch   string            `json:"epoch"`   // Epoch 表示 timeline 世代标识。
	NextSeq int64             `json:"nextSeq"` // NextSeq 表示下一行序号。
	Rows    []ChatTimelineRow `json:"rows"`    // Rows 表示已追加的行。
}

// cloneChatTimelineState 使用 timeline 参数创建副本。
func cloneChatTimelineState(timeline chatTimelineState) chatTimelineState {
	timeline.Rows = cloneChatTimelineRows(timeline.Rows)
	return timeline
}

// cloneChatTimelineRows 使用 rows 参数创建副本。
func cloneChatTimelineRows(rows []ChatTimelineRow) []ChatTimelineRow {
	if len(rows) == 0 {
		return nil
	}
	result := make([]ChatTimelineRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, cloneChatTimelineRow(row))
	}
	return result
}

// cloneChatTimelineRow 使用 row 参数创建副本。
func cloneChatTimelineRow(row ChatTimelineRow) ChatTimelineRow {
	row.Item = cloneChatTimelineItem(row.Item)
	return row
}

// cloneChatTimelineItem 使用 item 参数创建副本。
func cloneChatTimelineItem(item ChatTimelineItem) ChatTimelineItem {
	if item.ToolCall != nil {
		tool := cloneToolCall(*item.ToolCall)
		item.ToolCall = &tool
	}
	if item.Usage != nil {
		usage := *item.Usage
		item.Usage = &usage
	}
	if item.Plan != nil {
		plan := *item.Plan
		item.Plan = &plan
	}
	item.Images = append([]MessageImage(nil), item.Images...)
	return item
}

// newChatTimelineState 使用 chatID 参数创建空 timeline。
func newChatTimelineState(chatID string) chatTimelineState {
	return chatTimelineState{
		ChatID:  chatID,
		Epoch:   newID("epoch"),
		NextSeq: 1,
		Rows:    []ChatTimelineRow{},
	}
}

// nextTimelineSeq 使用 rows 参数返回下一条 timeline 序号。
func nextTimelineSeq(rows []ChatTimelineRow) int64 {
	if len(rows) == 0 {
		return 1
	}
	maxSeq := int64(0)
	for _, row := range rows {
		if row.Seq > maxSeq {
			maxSeq = row.Seq
		}
	}
	return maxSeq + 1
}

// appendChatTimelineRow 使用 timeline、item 和 now 参数追加一行。
func appendChatTimelineRow(timeline *chatTimelineState, item ChatTimelineItem, now time.Time) ChatTimelineRow {
	if strings.TrimSpace(timeline.Epoch) == "" {
		timeline.Epoch = newID("epoch")
	}
	if timeline.NextSeq <= 0 {
		timeline.NextSeq = 1
	}
	row := ChatTimelineRow{
		ID:        newID("tlrow"),
		ChatID:    timeline.ChatID,
		Epoch:     timeline.Epoch,
		Seq:       timeline.NextSeq,
		Timestamp: now,
		Item:      cloneChatTimelineItem(item),
	}
	timeline.NextSeq++
	timeline.Rows = append(timeline.Rows, row)
	return cloneChatTimelineRow(row)
}

// fetchChatTimelineRows 使用 timeline、direction、cursor 和 limit 参数读取 timeline 窗口。
func fetchChatTimelineRows(timeline chatTimelineState, direction string, cursor *ChatTimelineCursor, limit int) (ChatTimelineFetchResult, error) {
	direction = strings.TrimSpace(direction)
	if direction == "" {
		if cursor != nil {
			direction = ChatTimelineDirectionAfter
		} else {
			direction = ChatTimelineDirectionTail
		}
	}
	if direction != ChatTimelineDirectionTail && direction != ChatTimelineDirectionAfter && direction != ChatTimelineDirectionBefore {
		return ChatTimelineFetchResult{}, fmt.Errorf("%w: timeline direction 不合法: %s", ErrInvalidInput, direction)
	}
	if limit < 0 {
		return ChatTimelineFetchResult{}, fmt.Errorf("%w: timeline limit 不能为负数", ErrInvalidInput)
	}
	if limit == 0 {
		if direction == ChatTimelineDirectionAfter {
			limit = len(timeline.Rows)
		} else {
			limit = defaultChatTimelineFetchLimit
		}
	}
	if strings.TrimSpace(timeline.Epoch) == "" {
		timeline.Epoch = newID("epoch")
	}
	if timeline.NextSeq <= 0 {
		timeline.NextSeq = 1
	}
	minSeq, maxSeq := int64(0), int64(0)
	if len(timeline.Rows) > 0 {
		minSeq = timeline.Rows[0].Seq
		maxSeq = timeline.Rows[len(timeline.Rows)-1].Seq
	}
	window := ChatTimelineWindow{MinSeq: minSeq, MaxSeq: maxSeq, NextSeq: timeline.NextSeq}
	ctx := chatTimelineFetchContext{timeline: timeline, direction: direction, cursor: cursor, limit: limit, window: window}
	if cursor != nil && cursor.Epoch != "" && cursor.Epoch != timeline.Epoch {
		return chatTimelineResetResult(ctx, true, false), nil
	}
	if direction == ChatTimelineDirectionAfter && cursor != nil && len(timeline.Rows) > 0 && cursor.Seq < minSeq-1 {
		return chatTimelineResetResult(ctx, false, true), nil
	}
	switch direction {
	case ChatTimelineDirectionTail:
		return chatTimelineTailResult(ctx), nil
	case ChatTimelineDirectionAfter:
		return chatTimelineAfterResult(ctx), nil
	case ChatTimelineDirectionBefore:
		return chatTimelineBeforeResult(ctx), nil
	default:
		return ChatTimelineFetchResult{}, fmt.Errorf("%w: timeline direction 不合法: %s", ErrInvalidInput, direction)
	}
}

// chatTimelineFetchContext 表示 timeline 拉取内部上下文。
type chatTimelineFetchContext struct {
	timeline  chatTimelineState
	direction string
	cursor    *ChatTimelineCursor
	limit     int
	window    ChatTimelineWindow
}

// chatTimelineFetchResult 使用 ctx、rows 和标记参数构造响应。
func chatTimelineFetchResult(ctx chatTimelineFetchContext, rows []ChatTimelineRow, reset bool, staleCursor bool, gap bool, hasOlder bool, hasNewer bool) ChatTimelineFetchResult {
	var startCursor *ChatTimelineCursor
	var endCursor *ChatTimelineCursor
	if len(rows) > 0 {
		first := rows[0]
		last := rows[len(rows)-1]
		startCursor = &ChatTimelineCursor{Epoch: ctx.timeline.Epoch, Seq: first.Seq}
		endCursor = &ChatTimelineCursor{Epoch: ctx.timeline.Epoch, Seq: last.Seq}
	}
	return ChatTimelineFetchResult{
		ChatID:      ctx.timeline.ChatID,
		Direction:   ctx.direction,
		Epoch:       ctx.timeline.Epoch,
		Reset:       reset,
		StaleCursor: staleCursor,
		Gap:         gap,
		Window:      ctx.window,
		StartCursor: startCursor,
		EndCursor:   endCursor,
		HasOlder:    hasOlder,
		HasNewer:    hasNewer,
		Rows:        cloneChatTimelineRows(rows),
	}
}

// chatTimelineResetResult 使用 ctx 和标记参数返回 reset 窗口。
func chatTimelineResetResult(ctx chatTimelineFetchContext, staleCursor bool, gap bool) ChatTimelineFetchResult {
	rows := chatTimelineTailRows(ctx.timeline.Rows, ctx.limit)
	hasOlder := len(rows) > 0 && rows[0].Seq > ctx.window.MinSeq
	return chatTimelineFetchResult(ctx, rows, true, staleCursor, gap, hasOlder, false)
}

// chatTimelineTailResult 使用 ctx 参数返回 tail 窗口。
func chatTimelineTailResult(ctx chatTimelineFetchContext) ChatTimelineFetchResult {
	rows := chatTimelineTailRows(ctx.timeline.Rows, ctx.limit)
	hasOlder := len(rows) > 0 && rows[0].Seq > ctx.window.MinSeq
	return chatTimelineFetchResult(ctx, rows, false, false, false, hasOlder, false)
}

// chatTimelineAfterResult 使用 ctx 参数返回 cursor 之后窗口。
func chatTimelineAfterResult(ctx chatTimelineFetchContext) ChatTimelineFetchResult {
	baseSeq := int64(0)
	if ctx.cursor != nil {
		baseSeq = ctx.cursor.Seq
	}
	startIndex := sort.Search(len(ctx.timeline.Rows), func(index int) bool {
		return ctx.timeline.Rows[index].Seq > baseSeq
	})
	if startIndex >= len(ctx.timeline.Rows) {
		return chatTimelineFetchResult(ctx, []ChatTimelineRow{}, false, false, false, baseSeq >= ctx.window.MinSeq, false)
	}
	endIndex := startIndex + ctx.limit
	if endIndex > len(ctx.timeline.Rows) {
		endIndex = len(ctx.timeline.Rows)
	}
	rows := ctx.timeline.Rows[startIndex:endIndex]
	lastSeq := rows[len(rows)-1].Seq
	return chatTimelineFetchResult(ctx, rows, false, false, false, rows[0].Seq > ctx.window.MinSeq, lastSeq < ctx.window.MaxSeq)
}

// chatTimelineBeforeResult 使用 ctx 参数返回 cursor 之前窗口。
func chatTimelineBeforeResult(ctx chatTimelineFetchContext) ChatTimelineFetchResult {
	beforeSeq := ctx.timeline.NextSeq
	if ctx.cursor != nil && ctx.cursor.Seq > 0 {
		beforeSeq = ctx.cursor.Seq
	}
	endIndex := sort.Search(len(ctx.timeline.Rows), func(index int) bool {
		return ctx.timeline.Rows[index].Seq >= beforeSeq
	})
	if endIndex > len(ctx.timeline.Rows) {
		endIndex = len(ctx.timeline.Rows)
	}
	rows := ctx.timeline.Rows[:endIndex]
	if len(rows) > ctx.limit {
		rows = rows[len(rows)-ctx.limit:]
	}
	hasOlder := len(rows) > 0 && rows[0].Seq > ctx.window.MinSeq
	hasNewer := beforeSeq <= ctx.window.MaxSeq
	return chatTimelineFetchResult(ctx, rows, false, false, false, hasOlder, hasNewer)
}

// chatTimelineTailRows 使用 rows 和 limit 参数返回尾部窗口。
func chatTimelineTailRows(rows []ChatTimelineRow, limit int) []ChatTimelineRow {
	if len(rows) == 0 {
		return []ChatTimelineRow{}
	}
	if limit <= 0 || limit >= len(rows) {
		return rows
	}
	return rows[len(rows)-limit:]
}

// projectChatFromTimeline 使用 chat 和 rows 参数把 canonical timeline 投影为聊天正文。
func projectChatFromTimeline(chat Chat, rows []ChatTimelineRow) Chat {
	base := cloneChatSummary(chat)
	base.Messages = []ChatMessage{}
	base.Plan = nil
	base.Usage = nil
	messagesByID := make(map[string]int)
	for _, row := range rows {
		item := row.Item
		switch item.Type {
		case ChatTimelineItemMessageStarted, ChatTimelineItemSystemMessage:
			role := firstNonEmpty(item.Role, MessageRoleAssistant)
			if item.Type == ChatTimelineItemSystemMessage {
				role = MessageRoleSystem
			}
			message := ChatMessage{
				ID:        firstNonEmpty(item.MessageID, newID("msg")),
				ChatID:    row.ChatID,
				Role:      role,
				Text:      item.Text,
				Status:    firstNonEmpty(item.Status, MessageStatusComplete),
				Images:    append([]MessageImage(nil), item.Images...),
				CreatedAt: row.Timestamp,
				UpdatedAt: row.Timestamp,
			}
			messagesByID[message.ID] = len(base.Messages)
			base.Messages = append(base.Messages, message)
		case ChatTimelineItemAssistantDelta:
			index, ok := messagesByID[item.MessageID]
			if !ok {
				continue
			}
			message := &base.Messages[index]
			message.Text += item.Delta
			message.Status = MessageStatusStreaming
			message.UpdatedAt = row.Timestamp
			appendTextMessagePart(message, item.Delta, row)
		case ChatTimelineItemToolCall:
			if item.ToolCall == nil {
				continue
			}
			index, ok := messagesByID[item.MessageID]
			if !ok {
				continue
			}
			message := &base.Messages[index]
			tool := cloneToolCall(*item.ToolCall)
			upsertMessageTool(message, tool, row.Timestamp)
			message.UpdatedAt = row.Timestamp
		case ChatTimelineItemUsageUpdated:
			if item.Usage != nil {
				usage := *item.Usage
				base.Usage = &usage
			}
		case ChatTimelineItemMessageFinished:
			index, ok := messagesByID[item.MessageID]
			if !ok {
				continue
			}
			message := &base.Messages[index]
			message.Status = firstNonEmpty(item.Status, MessageStatusComplete)
			message.UpdatedAt = row.Timestamp
		case ChatTimelineItemPlanSet:
			if item.Plan != nil {
				plan := *item.Plan
				base.Plan = &plan
			}
		case ChatTimelineItemPlanStatusChanged:
			if base.Plan != nil && (item.Plan == nil || item.Plan.ID == base.Plan.ID) {
				base.Plan.Status = firstNonEmpty(item.Status, base.Plan.Status)
				base.Plan.UpdatedAt = row.Timestamp
			}
		}
	}
	return base
}

// appendTextMessagePart 使用 message、delta 和 row 参数追加文本片段。
func appendTextMessagePart(message *ChatMessage, delta string, row ChatTimelineRow) {
	if delta == "" {
		return
	}
	if len(message.Parts) > 0 && message.Parts[len(message.Parts)-1].Type == MessagePartTypeText {
		part := &message.Parts[len(message.Parts)-1]
		part.Text += delta
		part.UpdatedAt = row.Timestamp
		return
	}
	message.Parts = append(message.Parts, MessagePart{
		ID:        newID("part"),
		Type:      MessagePartTypeText,
		Text:      delta,
		CreatedAt: row.Timestamp,
		UpdatedAt: row.Timestamp,
	})
}

// upsertMessageTool 使用 message、tool 和 now 参数归并工具调用。
func upsertMessageTool(message *ChatMessage, tool ToolCall, now time.Time) {
	if tool.ID == "" {
		tool.ID = newID("tool")
	}
	if tool.Status == "" {
		tool.Status = ToolCallStatusRunning
	}
	for index := range message.ToolCalls {
		if message.ToolCalls[index].ID != tool.ID {
			continue
		}
		existing := &message.ToolCalls[index]
		existing.Name = firstNonEmpty(tool.Name, existing.Name)
		existing.Status = firstNonEmpty(tool.Status, existing.Status)
		existing.Input = firstNonEmpty(tool.Input, existing.Input)
		existing.Output = mergeToolOutput(*existing, tool)
		if tool.UserInputRequest != nil {
			request := cloneToolCall(tool).UserInputRequest
			existing.UserInputRequest = request
		}
		existing.UpdatedAt = now
		upsertMessageToolPart(message, *existing, now)
		return
	}
	tool.CreatedAt = now
	tool.UpdatedAt = now
	message.ToolCalls = append(message.ToolCalls, tool)
	upsertMessageToolPart(message, tool, now)
}
