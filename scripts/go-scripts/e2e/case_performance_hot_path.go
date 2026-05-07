package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/117503445/agenthub/internal/wsapp"
)

// runPerformanceHotPathCase 使用 ctx 参数验证流式输出热路径和恢复结果。
func runPerformanceHotPathCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "性能热路径 E2E 测试报告", success, events)
	}()

	fail := func(err error) bool {
		ctx.Logger.Errorf("性能热路径 E2E 失败: %v", err)
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
	if err := sendChatDetailLazyMessage(conn, "chat.agent.update", wsapp.ChatAgentUpdatePayload{
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
	if err := sendChatDetailLazyMessage(conn, "chat.detail.get", map[string]string{"chatId": chat.ID}); err != nil {
		return fail(err)
	}
	if _, err := readPersistenceMessage(conn, "chat.detail", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := sendChatDetailLazyMessage(conn, "chat.send", wsapp.ChatSendPayload{
		ChatID: chat.ID,
		Prompt: "MOCK_CODEX_DUPLICATE_FULL_TEXT",
	}); err != nil {
		return fail(err)
	}
	_, finalText, err := waitStreamCoalescingDone(conn, 20*time.Second)
	if err != nil {
		return fail(err)
	}
	if strings.Count(finalText, "Codex 重复输出修复完成") != 1 {
		return fail(fmt.Errorf("Codex 输出重复或缺失: %q", finalText))
	}
	if err := assertPerformanceDetailFile(ctx.DataDir, chat.ID, "Codex 重复输出修复完成"); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("Codex delta 和完整文本事件合并后只显示一次，聊天详情文件包含最终文本。"))

	conn.CloseNow()
	ctx.StopServer()
	restartBaseURL, restartStop, err := startPersistenceServer(ctx, "performance-restart-logs")
	if err != nil {
		return fail(err)
	}
	defer restartStop()
	if err := waitUntilReady(restartBaseURL); err != nil {
		return fail(fmt.Errorf("重启服务未就绪: %w", err))
	}
	restartConn, err := dialPersistenceWS(restartBaseURL)
	if err != nil {
		return fail(err)
	}
	defer restartConn.CloseNow()
	if _, err := readPersistenceMessage(restartConn, "state.snapshot", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := sendChatDetailLazyMessage(restartConn, "chat.detail.get", map[string]string{"chatId": chat.ID}); err != nil {
		return fail(err)
	}
	detailMessage, err := readPersistenceMessage(restartConn, "chat.detail", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	detail, err := decodeChatDetailLazyDetail(detailMessage.Payload)
	if err != nil {
		return fail(err)
	}
	if !chatDetailHasAssistantText(detail.Chat, "Codex 重复输出修复完成") {
		return fail(fmt.Errorf("重启后聊天详情缺少最终文本: %#v", detail.Chat.Messages))
	}
	events = append(events, reportStep("服务重启后，聊天详情仍能恢复完整 assistant 文本。"))
	return true
}

// assertPerformanceDetailFile 使用 dataDir、chatID 和 expected 参数断言详情文件包含最终文本。
func assertPerformanceDetailFile(dataDir string, chatID string, expected string) error {
	data, err := os.ReadFile(filepath.Join(dataDir, "chats", chatID+".json"))
	if err != nil {
		return err
	}
	var detail wsapp.PersistedChatDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return err
	}
	for _, message := range detail.Messages {
		if message.Role == wsapp.MessageRoleAssistant && strings.Contains(message.Text, expected) {
			return nil
		}
	}
	return fmt.Errorf("详情文件缺少 assistant 最终文本: %s", expected)
}

// chatDetailHasAssistantText 使用 chat 和 expected 参数判断聊天详情是否包含 assistant 文本。
func chatDetailHasAssistantText(chat wsapp.Chat, expected string) bool {
	for _, message := range chat.Messages {
		if message.Role == wsapp.MessageRoleAssistant && strings.Contains(message.Text, expected) {
			return true
		}
	}
	return false
}
