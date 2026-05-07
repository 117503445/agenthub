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

	"github.com/117503445/agenthub/internal/buildinfo"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog/log"
)

// Server 提供项目、聊天和 agent 流式输出的 WebSocket 服务。
type Server struct {
	ctx             context.Context
	version         string
	buildTime       string
	hostname        string
	store           *Store
	agents          *AgentManager
	agentConfig     AgentConfig
	subscribers     map[string]*serverSubscriber
	lastAgentSkills []AgentSkillOption
	mu              sync.Mutex
	skillsMu        sync.Mutex
}

// serverSubscriber 表示单个 WebSocket 连接的服务端订阅状态。
type serverSubscriber struct {
	outbound     chan ServerMessage // outbound 表示该连接的写出消息队列。
	activeChatID string             // activeChatID 表示该连接当前进入的聊天页。
}

// NewServer 使用 ctx、version 和 agentConfig 参数创建 WebSocket 服务。
func NewServer(ctx context.Context, version string, agentConfig AgentConfig) *Server {
	agentProfiles := agentConfig.AgentProfiles
	if len(agentProfiles) == 0 {
		agentProfiles = AgentProfiles(AgentOptionsConfig{})
	}
	store := NewStoreWithAgentProfiles(agentProfiles)
	addWorkdirProjectIfGitRepo(ctx, store)
	return newServerWithStore(ctx, version, agentConfig, store)
}

// NewPersistentServer 使用 ctx、version、agentConfig 和 dataDir 参数创建带持久化的 WebSocket 服务。
func NewPersistentServer(ctx context.Context, version string, agentConfig AgentConfig, dataDir string) (*Server, error) {
	agentProfiles := agentConfig.AgentProfiles
	if len(agentProfiles) == 0 {
		agentProfiles = AgentProfiles(AgentOptionsConfig{})
	}
	store, err := NewPersistentStore(dataDir, agentProfiles)
	if err != nil {
		return nil, err
	}
	addWorkdirProjectIfGitRepo(ctx, store)
	return newServerWithStore(ctx, version, agentConfig, store), nil
}

// newServerWithStore 使用 ctx、version、agentConfig 和 store 参数创建 WebSocket 服务。
func newServerWithStore(ctx context.Context, version string, agentConfig AgentConfig, store *Store) *Server {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown"
	}
	agentConfig.AgentProfiles = store.AgentProfiles()
	agentConfig.AgentProviders = store.AgentProviders()
	if len(agentConfig.BackendEnv) == 0 {
		agentConfig.BackendEnv = BackendEnvSnapshot()
	}
	return &Server{
		ctx:             ctx,
		version:         version,
		buildTime:       buildinfo.BuildTime,
		hostname:        hostname,
		store:           store,
		agents:          NewAgentManager(ctx, agentConfig),
		agentConfig:     agentConfig,
		subscribers:     make(map[string]*serverSubscriber),
		lastAgentSkills: store.AgentSkills(),
	}
}

// addWorkdirProjectIfGitRepo 使用 ctx 和 store 参数把后端 Git 工作目录加入初始 project 列表。
func addWorkdirProjectIfGitRepo(ctx context.Context, store *Store) {
	workdir, err := os.Getwd()
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Msg("读取后端工作目录失败")
		return
	}
	project, chat, created, err := store.CreateProjectFromGitWorkdir(workdir)
	if err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("workdir", workdir).Msg("自动添加工作目录 project 失败")
		return
	}
	if !created {
		return
	}
	log.Ctx(ctx).Info().
		Str("projectID", project.ID).
		Str("chatID", chat.ID).
		Str("workdir", project.Path).
		Msg("已自动添加后端工作目录 project")
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

		if err := s.handle(ctx, subscriberID, outbound, msg); err != nil {
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
func (s *Server) handle(ctx context.Context, subscriberID string, outbound chan ServerMessage, msg ClientMessage) error {
	switch msg.Type {
	case "ping":
		s.sendTo(outbound, "pong", map[string]any{"message": "pong"})
		return nil
	case "agent.skills.refresh":
		s.refreshAgentSkills(s.ctx)
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
		s.broadcastChatChanged(chat)
		s.broadcastAgentSkills()
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
		s.broadcastAgentSkills()
		return nil
	case "project.reorder":
		var payload ProjectReorderPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		projects, err := s.store.ReorderProjects(payload.ProjectIDs)
		if err != nil {
			return err
		}
		s.broadcast("projects.reordered", map[string]any{"projects": projects})
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
		s.broadcastAgentSkills()
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
		s.broadcastChatChanged(chat)
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
		s.broadcastChatChanged(chat)
		return nil
	case "chat.detail.get":
		var payload ChatDetailGetPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		chat, err := s.store.GetChat(payload.ChatID)
		if err != nil {
			return err
		}
		s.setSubscriberActiveChat(subscriberID, chat.ID)
		s.sendTo(outbound, "chat.detail", map[string]any{"chat": chat})
		return nil
	case "chat.draft.update":
		var payload ChatDraftUpdatePayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		chat, changed, err := s.store.UpdateChatDraft(payload.ChatID, payload.Text)
		if err != nil {
			return err
		}
		if changed {
			s.broadcastChatChanged(chat)
		}
		return nil
	case "agent.model.add":
		var payload AgentModelAddPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		options, err := s.store.AddAgentModel(payload.Provider, payload.ID)
		if err != nil {
			return err
		}
		s.agents.SetAgentProfiles(s.store.AgentProfiles())
		s.broadcast("agent.providers.changed", map[string]any{"agentProviders": options})
		s.broadcastAgentProfilesChanged()
		return nil
	case "agent.profile.create":
		var payload AgentProfile
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		if _, err := s.store.CreateAgentProfile(payload); err != nil {
			return err
		}
		s.broadcastAgentProfilesChanged()
		return nil
	case "agent.profile.update":
		var payload AgentProfile
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		if _, err := s.store.UpdateAgentProfile(payload); err != nil {
			return err
		}
		s.broadcastAgentProfilesChanged()
		return nil
	case "agent.profile.delete":
		var payload IDPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		if _, err := s.store.DeleteAgentProfile(payload.ID); err != nil {
			return err
		}
		s.broadcastAgentProfilesChanged()
		return nil
	case "agent.profile.add_builtin":
		var payload AgentBuiltinProfilePayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		profile, err := BuiltinAgentProfile(payload.Kind, s.agentOptionsConfig(), s.store.AgentProfiles())
		if err != nil {
			return err
		}
		if _, err := s.store.CreateAgentProfile(profile); err != nil {
			return err
		}
		s.broadcastAgentProfilesChanged()
		return nil
	case "agent.profile.model.add":
		var payload AgentProfileModelPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		if _, err := s.store.AddAgentProfileModel(payload.ProfileID, payload.ID); err != nil {
			return err
		}
		s.broadcastAgentProfilesChanged()
		return nil
	case "agent.profile.model.update":
		var payload AgentProfileModelPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		if _, err := s.store.UpdateAgentProfileModel(payload.ProfileID, payload.ID, payload.Default); err != nil {
			return err
		}
		s.broadcastAgentProfilesChanged()
		return nil
	case "agent.profile.model.delete":
		var payload AgentProfileModelPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		if _, err := s.store.DeleteAgentProfileModel(payload.ProfileID, payload.ID); err != nil {
			return err
		}
		s.broadcastAgentProfilesChanged()
		return nil
	case "agent.profile.effective_env.get":
		var payload IDPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		profile, ok := AgentProfileByID(s.store.AgentProfiles(), payload.ID)
		if !ok {
			return ErrNotFound
		}
		s.sendTo(outbound, "agent.profile.effective_env", map[string]any{
			"id":  profile.ID,
			"env": EffectiveAgentEnv(s.agentConfig.BackendEnv, profile),
		})
		return nil
	case "chat.send":
		var payload ChatSendPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		s.setSubscriberActiveChat(subscriberID, payload.ChatID)
		if isAgentSkillsCommand(payload.Prompt) {
			return s.respondAgentSkillsCommand(ctx, payload.ChatID, payload.Prompt)
		}
		return s.startChatRun(ctx, payload.ChatID, payload.Prompt, payload.Images, payload.PlanMode)
	case "chat.plan.execute":
		var payload ChatPlanExecutePayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		s.setSubscriberActiveChat(subscriberID, payload.ChatID)
		return s.startPlanExecution(ctx, payload.ChatID, payload.PlanID)
	case "chat.user_input.respond":
		var payload ChatUserInputRespondPayload
		if err := decodePayload(msg, &payload); err != nil {
			return err
		}
		s.setSubscriberActiveChat(subscriberID, payload.ChatID)
		return s.agents.RespondUserInput(payload.ChatID, payload.ToolCallID, payload.Answers)
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

// respondAgentSkillsCommand 使用 ctx、chatID 和 prompt 参数在聊天框本地回复 skills 列表。
func (s *Server) respondAgentSkillsCommand(ctx context.Context, chatID string, prompt string) error {
	skills := s.store.AgentSkills()
	s.setLastAgentSkills(skills)
	response := formatAgentSkillsMarkdown(skills)
	chat, _, assistantMessage, err := s.store.AddLocalAssistantResponse(chatID, prompt, response)
	if err != nil {
		return err
	}
	log.Ctx(ctx).Info().
		Str("chatID", chatID).
		Int("skillCount", len(skills)).
		Msg("已本地回复 skills 列表")
	s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusRunning})
	s.broadcastChatChanged(chat)
	s.broadcastChatDetailChanged(chatID, chat)
	s.broadcastToActiveChat(chatID, "chat.message.done", map[string]any{"chatId": chatID, "message": assistantMessage})
	s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusIdle})
	return nil
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
	s.broadcastChatChanged(chat)
	s.broadcastChatDetailChanged(chatID, chat)
	s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusRunning})

	deltaCoalescer := newAssistantDeltaCoalescer(60*time.Millisecond, func(delta string) {
		message, ok := s.store.AppendAssistantDelta(chatID, assistantMessage.ID, delta)
		if !ok {
			return
		}
		s.broadcastToActiveChat(chatID, "chat.message.delta", map[string]any{
			"chatId":    chatID,
			"messageId": message.ID,
			"delta":     delta,
			"text":      message.Text,
			"message":   message,
		})
	})
	callbacks := AgentRunCallbacks{
		OnSessionID: func(sessionID string) {
			if updatedChat, ok := s.store.SetChatSessionID(chatID, sessionID); ok {
				s.broadcastChatChanged(updatedChat)
			}
		},
		OnDelta: func(delta string) {
			deltaCoalescer.Add(delta)
		},
		OnToolCall: func(tool ToolCall) {
			deltaCoalescer.Flush()
			updatedChat, _, ok := s.store.UpsertToolCall(chatID, assistantMessage.ID, tool)
			if !ok {
				return
			}
			s.broadcastChatChanged(updatedChat)
			s.broadcastChatDetailChanged(chatID, updatedChat)
		},
		OnUsage: func(usage AgentUsage) {
			if updatedChat, ok := s.store.SetChatUsage(chatID, usage); ok {
				s.broadcastChatChanged(updatedChat)
				s.broadcastChatDetailChanged(chatID, updatedChat)
			}
		},
		OnDone: func() {
			deltaCoalescer.Close()
			updatedChat, message, ok := s.store.FinishAssistantMessage(chatID, assistantMessage.ID, MessageStatusComplete)
			if !ok {
				return
			}
			if planMode {
				if planChat, planOK := s.store.SetChatPlan(chatID, assistantMessage.ID, message.Text); planOK {
					updatedChat = planChat
				}
			}
			s.broadcastToActiveChat(chatID, "chat.message.done", map[string]any{"chatId": chatID, "message": message})
			s.broadcastChatChanged(updatedChat)
			s.broadcastChatDetailChanged(chatID, updatedChat)
			s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusIdle})
		},
		OnError: func(message string) {
			deltaCoalescer.Close()
			updatedChat, assistant, ok := s.store.FinishAssistantMessage(chatID, assistantMessage.ID, MessageStatusError)
			if ok {
				s.broadcastToActiveChat(chatID, "chat.message.done", map[string]any{"chatId": chatID, "message": assistant})
				s.broadcastChatChanged(updatedChat)
				s.broadcastChatDetailChanged(chatID, updatedChat)
			}
			if errorChat, systemMessage, err := s.store.AddSystemMessage(chatID, message, MessageStatusError); err == nil {
				s.broadcastChatChanged(errorChat)
				s.broadcastChatDetailChanged(chatID, errorChat)
				s.broadcastToActiveChat(chatID, "chat.message.done", map[string]any{"chatId": chatID, "message": systemMessage})
			}
			s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusError})
		},
	}

	if err := s.agents.Send(ctx, AgentRunInput{
		ChatID:             chatID,
		ProjectPath:        project.Path,
		Provider:           chat.AgentProvider,
		Profile:            chat.AgentProfile,
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
	s.broadcastChatChanged(chat)
	s.broadcastChatDetailChanged(chatID, chat)
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
		s.broadcastToActiveChat(chatID, "chat.message.done", map[string]any{"chatId": chatID, "message": message})
	}
	s.broadcastChatChanged(chat)
	s.broadcastChatDetailChanged(chatID, chat)
	s.broadcast("agent.status", map[string]any{"chatId": chatID, "status": ChatStatusIdle})
}

// broadcastAgentProfilesChanged 广播 Profile 和兼容 provider 列表变更。
func (s *Server) broadcastAgentProfilesChanged() {
	profiles := s.store.AgentProfiles()
	providers := AgentProviderOptionsFromProfiles(profiles)
	s.agents.SetAgentProfiles(profiles)
	s.broadcast("agent.profiles.changed", map[string]any{
		"agentProfiles":  profiles,
		"agentProviders": providers,
	})
	s.broadcast("agent.providers.changed", map[string]any{"agentProviders": providers})
}

// agentOptionsConfig 返回当前服务重新添加内置 Profile 所需的启动配置。
func (s *Server) agentOptionsConfig() AgentOptionsConfig {
	return AgentOptionsConfig{
		ClaudeCommand:        s.agentConfig.Command,
		CodexCommand:         s.agentConfig.CodexCommand,
		MockClaudeCommand:    s.agentConfig.MockClaudeCommand,
		MockCodexCommand:     s.agentConfig.MockCodexCommand,
		MockAnthropicBaseURL: s.agentConfig.MockAnthropicBaseURL,
		MockAnthropicAPIKey:  s.agentConfig.MockAnthropicAPIKey,
		MockOpenAIBaseURL:    s.agentConfig.MockOpenAIBaseURL,
		MockOpenAIAPIKey:     s.agentConfig.MockOpenAIAPIKey,
		EnableMockAgent:      s.agentConfig.EnableMockAgent,
	}
}

// refreshAgentSkills 使用 ctx 参数刷新 skill 列表，并在列表变化时广播和记录日志。
func (s *Server) refreshAgentSkills(ctx context.Context) {
	skills := s.store.AgentSkills()
	if !s.updateLastAgentSkills(skills) {
		return
	}
	log.Ctx(ctx).Info().
		Int("skillCount", len(skills)).
		Strs("skills", agentSkillIDs(skills)).
		Msg("agent skills 列表已更新")
	s.broadcast("agent.skills.changed", map[string]any{"agentSkills": skills})
}

// broadcastAgentSkills 读取当前 skill 列表并广播给前端。
func (s *Server) broadcastAgentSkills() {
	skills := s.store.AgentSkills()
	s.setLastAgentSkills(skills)
	s.broadcast("agent.skills.changed", map[string]any{"agentSkills": skills})
}

// updateLastAgentSkills 使用 skills 参数更新上一次 skill 列表，并返回列表是否变化。
func (s *Server) updateLastAgentSkills(skills []AgentSkillOption) bool {
	s.skillsMu.Lock()
	defer s.skillsMu.Unlock()
	if agentSkillOptionsEqual(s.lastAgentSkills, skills) {
		return false
	}
	s.lastAgentSkills = cloneAgentSkillOptions(skills)
	return true
}

// setLastAgentSkills 使用 skills 参数更新服务端缓存的上一次 skill 列表。
func (s *Server) setLastAgentSkills(skills []AgentSkillOption) {
	s.skillsMu.Lock()
	s.lastAgentSkills = cloneAgentSkillOptions(skills)
	s.skillsMu.Unlock()
}

// cloneAgentSkillOptions 使用 skills 参数复制 skill 列表。
func cloneAgentSkillOptions(skills []AgentSkillOption) []AgentSkillOption {
	return append([]AgentSkillOption(nil), skills...)
}

// agentSkillIDs 使用 skills 参数返回日志中展示的 skill 标识列表。
func agentSkillIDs(skills []AgentSkillOption) []string {
	ids := make([]string, 0, len(skills))
	for _, skill := range skills {
		ids = append(ids, skill.ID)
	}
	return ids
}

// agentSkillOptionsEqual 使用 left 和 right 参数判断两个 skill 列表是否完全一致。
func agentSkillOptionsEqual(left []AgentSkillOption, right []AgentSkillOption) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// subscribe 创建新的消息订阅通道。
func (s *Server) subscribe() (string, chan ServerMessage) {
	subscriberID := newID("subscriber")
	ch := make(chan ServerMessage, 256)
	s.mu.Lock()
	s.subscribers[subscriberID] = &serverSubscriber{outbound: ch}
	s.mu.Unlock()
	return subscriberID, ch
}

// unsubscribe 使用 subscriberID 参数取消消息订阅。
func (s *Server) unsubscribe(subscriberID string) {
	s.mu.Lock()
	subscriber, ok := s.subscribers[subscriberID]
	if ok {
		delete(s.subscribers, subscriberID)
		close(subscriber.outbound)
	}
	s.mu.Unlock()
}

// setSubscriberActiveChat 使用 subscriberID 和 chatID 参数记录连接当前进入的聊天页。
func (s *Server) setSubscriberActiveChat(subscriberID string, chatID string) {
	s.mu.Lock()
	if subscriber, ok := s.subscribers[subscriberID]; ok {
		subscriber.activeChatID = chatID
	}
	s.mu.Unlock()
}

// broadcast 使用 messageType 和 payload 参数广播服务端消息。
func (s *Server) broadcast(messageType string, payload any) {
	message := s.message(messageType, payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, subscriber := range s.subscribers {
		select {
		case subscriber.outbound <- message:
		default:
			log.Ctx(s.ctx).Warn().Str("type", messageType).Msg("订阅者消息队列已满，丢弃消息")
		}
	}
}

// broadcastToActiveChat 使用 chatID、messageType 和 payload 参数向已进入该聊天页的连接发送消息。
func (s *Server) broadcastToActiveChat(chatID string, messageType string, payload any) {
	message := s.message(messageType, payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, subscriber := range s.subscribers {
		if subscriber.activeChatID != chatID {
			continue
		}
		select {
		case subscriber.outbound <- message:
		default:
			log.Ctx(s.ctx).Warn().Str("type", messageType).Str("chatID", chatID).Msg("聊天详情订阅者消息队列已满，丢弃消息")
		}
	}
}

// broadcastChatChanged 使用 chat 参数向所有连接广播聊天页摘要变更。
func (s *Server) broadcastChatChanged(chat Chat) {
	s.broadcast("chat.changed", map[string]any{"chat": cloneChatSummary(chat)})
}

// broadcastChatDetailChanged 使用 chatID 和 chat 参数向已进入聊天页的连接广播完整详情变更。
func (s *Server) broadcastChatDetailChanged(chatID string, chat Chat) {
	s.broadcastToActiveChat(chatID, "chat.detail.changed", map[string]any{"chat": cloneChat(chat)})
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
		BuildTime:  s.buildTime,
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
