package wsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog/log"
)

// Server 提供项目、聊天和 agent 流式输出的 WebSocket 服务。
type Server struct {
	ctx         context.Context
	version     string
	hostname    string
	store       *Store
	agents      *AgentManager
	subscribers map[string]chan ServerMessage
	mu          sync.Mutex
}

// NewServer 使用 ctx、version 和 agentConfig 参数创建 WebSocket 服务。
func NewServer(ctx context.Context, version string, agentConfig AgentConfig) *Server {
	agentProviders := agentConfig.AgentProviders
	if len(agentProviders) == 0 {
		agentProviders = DefaultAgentProviderOptions()
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown"
	}
	return &Server{
		ctx:         ctx,
		version:     version,
		hostname:    hostname,
		store:       NewStoreWithAgentProviders(agentProviders),
		agents:      NewAgentManager(ctx, agentConfig),
		subscribers: make(map[string]chan ServerMessage),
	}
}

// ServeWS 使用 w 和 r 参数升级 HTTP 请求并处理 WebSocket 消息。
func (s *Server) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
	})
	if err != nil {
		log.Ctx(r.Context()).Error().Err(err).Msg("接受 WebSocket 连接失败")
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(16 * 1024 * 1024)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	subscriberID, outbound := s.subscribe()
	defer s.unsubscribe(subscriberID)
	s.sendTo(outbound, "state.snapshot", s.store.Snapshot())

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- s.writeLoop(ctx, conn, outbound)
	}()

	for {
		select {
		case err := <-writeDone:
			if err != nil && !isExpectedClose(err) {
				log.Ctx(ctx).Error().Err(err).Msg("写入 WebSocket 消息失败")
			}
			return
		default:
		}

		var msg ClientMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			if isExpectedClose(err) {
				log.Ctx(ctx).Info().Msg("WebSocket 连接已关闭")
				return
			}
			log.Ctx(ctx).Error().Err(err).Msg("读取 WebSocket 消息失败")
			return
		}

		if err := s.handle(ctx, outbound, msg); err != nil {
			log.Ctx(ctx).Error().Err(err).Str("type", msg.Type).Msg("处理 WebSocket 消息失败")
			s.sendTo(outbound, "error", map[string]any{
				"message": err.Error(),
				"type":    msg.Type,
			})
		}
	}
}

// writeLoop 使用 ctx、conn 和 outbound 参数持续写出服务端消息。
func (s *Server) writeLoop(ctx context.Context, conn *websocket.Conn, outbound <-chan ServerMessage) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-outbound:
			if !ok {
				return nil
			}
			if err := wsjson.Write(ctx, conn, message); err != nil {
				return err
			}
		}
	}
}

// handle 根据 msg 参数中的类型处理连接上的业务消息。
func (s *Server) handle(ctx context.Context, outbound chan ServerMessage, msg ClientMessage) error {
	switch msg.Type {
	case "ping":
		s.sendTo(outbound, "pong", map[string]any{"message": "pong"})
		return nil
	case "project.create":
		var payload ProjectMutationPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		project, err := s.store.CreateProject(payload.Path)
		if err != nil {
			return err
		}
		chat, err := s.store.CreateChat(project.ID)
		if err != nil {
			return err
		}
		s.broadcast("project.changed", map[string]any{"project": project})
		s.broadcast("chat.changed", map[string]any{"chat": chat})
		s.broadcast("agent.skills.changed", map[string]any{"agentSkills": s.store.AgentSkills()})
		return nil
	case "project.update":
		var payload ProjectMutationPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		project, err := s.store.UpdateProject(payload.ID, payload.Path)
		if err != nil {
			return err
		}
		s.broadcast("project.changed", map[string]any{"project": project})
		s.broadcast("agent.skills.changed", map[string]any{"agentSkills": s.store.AgentSkills()})
		return nil
	case "project.delete":
		var payload IDPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		chatIDs, err := s.store.DeleteProject(payload.ID)
		if err != nil {
			return err
		}
		for _, chatID := range chatIDs {
			s.agents.Stop(chatID)
		}
		s.broadcast("project.deleted", map[string]any{"id": payload.ID, "chatIds": chatIDs})
		s.broadcast("agent.skills.changed", map[string]any{"agentSkills": s.store.AgentSkills()})
		return nil
	case "chat.create":
		var payload ChatCreatePayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		chat, err := s.store.CreateChat(payload.ProjectID)
		if err != nil {
			return err
		}
		s.broadcast("chat.changed", map[string]any{"chat": chat})
		return nil
	case "chat.delete":
		var payload IDPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		projectID, err := s.store.DeleteChat(payload.ID)
		if err != nil {
			return err
		}
		s.agents.Stop(payload.ID)
		s.broadcast("chat.deleted", map[string]any{"id": payload.ID, "projectId": projectID})
		return nil
	case "chat.agent.update":
		var payload ChatAgentUpdatePayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		chat, err := s.store.UpdateChatAgent(payload.ChatID, payload.Provider, payload.Model, payload.Reasoning)
		if err != nil {
			return err
		}
		if chat.Status == ChatStatusRunning {
			s.stopChatRun(payload.ChatID)
			return nil
		}
		s.agents.Stop(payload.ChatID)
		s.broadcast("chat.changed", map[string]any{"chat": chat})
		return nil
	case "agent.model.add":
		var payload AgentModelAddPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		options, err := s.store.AddAgentModel(payload.Provider, payload.ID, payload.Label)
		if err != nil {
			return err
		}
		s.agents.SetAgentProviders(options)
		s.broadcast("agent.providers.changed", map[string]any{"agentProviders": options})
		return nil
	case "chat.send":
		var payload ChatSendPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		return s.startChatRun(ctx, payload.ChatID, payload.Prompt, payload.Images, payload.PlanMode)
	case "chat.plan.execute":
		var payload ChatPlanExecutePayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		return s.startPlanExecution(ctx, payload.ChatID, payload.PlanID)
	case "chat.stop":
		var payload ChatStopPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		s.stopChatRun(payload.ChatID)
		return nil
	default:
		return fmt.Errorf("不支持的消息类型: %s", msg.Type)
	}
}

// startChatRun 使用 ctx、chatID 和 prompt 参数启动或替换聊天页 agent 输出。
func (s *Server) startChatRun(ctx context.Context, chatID string, prompt string, images []MessageImagePayload, planMode bool) error {
	project, chat, err := s.store.GetProjectAndChat(chatID)
	if err != nil {
		return err
	}
	if chat.Status == ChatStatusRunning {
		s.stopChatRun(chatID)
	}

	chat, userMessage, assistantMessage, err := s.store.AddRunMessages(chatID, prompt, images, planMode)
	if err != nil {
		return err
	}
	s.broadcast("chat.changed", map[string]any{"chat": chat})
	s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusRunning})

	callbacks := AgentRunCallbacks{
		OnSessionID: func(sessionID string) {
			if updatedChat, ok := s.store.SetChatSessionID(chatID, sessionID); ok {
				s.broadcast("chat.changed", map[string]any{"chat": updatedChat})
			}
		},
		OnDelta: func(delta string) {
			message, ok := s.store.AppendAssistantDelta(chatID, assistantMessage.ID, delta)
			if !ok {
				return
			}
			s.broadcast("chat.message.delta", map[string]any{
				"chatId":    chatID,
				"messageId": message.ID,
				"delta":     delta,
				"text":      message.Text,
				"message":   message,
			})
		},
		OnToolCall: func(tool ToolCall) {
			updatedChat, _, ok := s.store.UpsertToolCall(chatID, assistantMessage.ID, tool)
			if !ok {
				return
			}
			s.broadcast("chat.changed", map[string]any{"chat": updatedChat})
		},
		OnUsage: func(usage ContextWindowUsage) {
			updatedChat, ok := s.store.UpdateContextWindowUsage(chatID, usage)
			if ok {
				s.broadcast("chat.changed", map[string]any{"chat": updatedChat})
			}
		},
		OnDone: func() {
			updatedChat, message, ok := s.store.FinishAssistantMessage(chatID, assistantMessage.ID, MessageStatusComplete)
			if !ok {
				return
			}
			if planMode {
				if planChat, planOK := s.store.SetChatPlan(chatID, assistantMessage.ID, message.Text); planOK {
					updatedChat = planChat
				}
			}
			s.broadcast("chat.message.done", map[string]any{"chatId": chatID, "message": message})
			s.broadcast("chat.changed", map[string]any{"chat": updatedChat})
			s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusIdle})
		},
		OnError: func(message string) {
			updatedChat, assistant, ok := s.store.FinishAssistantMessage(chatID, assistantMessage.ID, MessageStatusError)
			if ok {
				s.broadcast("chat.message.done", map[string]any{"chatId": chatID, "message": assistant})
				s.broadcast("chat.changed", map[string]any{"chat": updatedChat})
			}
			if errorChat, systemMessage, err := s.store.AddSystemMessage(chatID, message, MessageStatusError); err == nil {
				s.broadcast("chat.changed", map[string]any{"chat": errorChat})
				s.broadcast("chat.message.done", map[string]any{"chatId": chatID, "message": systemMessage})
			}
			s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusError})
		},
	}

	if err := s.agents.Send(ctx, AgentRunInput{
		ChatID:             chatID,
		ProjectPath:        project.Path,
		Provider:           chat.AgentProvider,
		Model:              chat.AgentModel,
		Reasoning:          chat.AgentReasoning,
		Prompt:             prompt,
		Images:             userMessage.Images,
		PlanMode:           planMode,
		SessionID:          chat.AgentSessionID,
		AssistantMessageID: assistantMessage.ID,
		Callbacks:          callbacks,
	}); err != nil {
		callbacks.OnError(err.Error())
		return err
	}
	return nil
}

// startPlanExecution 使用 ctx、chatID 和 planID 参数执行已确认 plan。
func (s *Server) startPlanExecution(ctx context.Context, chatID string, planID string) error {
	chat, plan, err := s.store.MarkPlanExecuting(chatID, planID)
	if err != nil {
		return err
	}
	s.broadcast("chat.changed", map[string]any{"chat": chat})
	prompt := strings.Join([]string{
		"开始执行已确认的 plan。",
		"",
		"已确认 plan:",
		plan.Text,
		"",
		"请按该 plan 开始实现，不要重新生成 plan，除非遇到阻塞。",
	}, "\n")
	return s.startChatRun(ctx, chatID, prompt, nil, false)
}

// stopChatRun 使用 chatID 参数停止聊天页当前输出。
func (s *Server) stopChatRun(chatID string) {
	s.agents.Stop(chatID)
	chat, message, ok := s.store.StopStreamingMessage(chatID, MessageStatusStopped)
	if !ok {
		return
	}
	if message.ID != "" {
		s.broadcast("chat.message.done", map[string]any{"chatId": chatID, "message": message})
	}
	s.broadcast("chat.changed", map[string]any{"chat": chat})
	s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusIdle})
}

// subscribe 创建新的消息订阅通道。
func (s *Server) subscribe() (string, chan ServerMessage) {
	subscriberID := newID("subscriber")
	ch := make(chan ServerMessage, 256)
	s.mu.Lock()
	s.subscribers[subscriberID] = ch
	s.mu.Unlock()
	return subscriberID, ch
}

// unsubscribe 使用 subscriberID 参数取消消息订阅。
func (s *Server) unsubscribe(subscriberID string) {
	s.mu.Lock()
	ch, ok := s.subscribers[subscriberID]
	if ok {
		delete(s.subscribers, subscriberID)
		close(ch)
	}
	s.mu.Unlock()
}

// broadcast 使用 messageType 和 payload 参数广播服务端消息。
func (s *Server) broadcast(messageType string, payload any) {
	message := s.message(messageType, payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- message:
		default:
			log.Ctx(s.ctx).Warn().Str("type", messageType).Msg("订阅者消息队列已满，丢弃消息")
		}
	}
}

// sendTo 使用 ch、messageType 和 payload 参数发送单个服务端消息。
func (s *Server) sendTo(ch chan ServerMessage, messageType string, payload any) {
	message := s.message(messageType, payload)
	select {
	case ch <- message:
	default:
		log.Ctx(s.ctx).Warn().Str("type", messageType).Msg("WebSocket 发送队列已满，丢弃消息")
	}
}

// message 使用 messageType 和 payload 参数构造统一服务端消息。
func (s *Server) message(messageType string, payload any) ServerMessage {
	return ServerMessage{
		Type:       messageType,
		Payload:    payload,
		ServerTime: time.Now().Format(time.RFC3339),
		Version:    s.version,
		Hostname:   s.hostname,
	}
}

// decodePayload 使用 msg 和 target 参数解析客户端消息 payload。
func decodePayload(msg ClientMessage, target any) error {
	if len(msg.Payload) == 0 {
		return fmt.Errorf("%w: payload 不能为空", ErrInvalidInput)
	}
	if err := json.Unmarshal(msg.Payload, target); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return nil
}

// isExpectedClose 判断 err 参数是否属于正常关闭。
func isExpectedClose(err error) bool {
	if err == nil {
		return true
	}
	status := websocket.CloseStatus(err)
	return errors.Is(err, context.Canceled) ||
		status == websocket.StatusNormalClosure ||
		status == websocket.StatusGoingAway ||
		status == websocket.StatusNoStatusRcvd
}
