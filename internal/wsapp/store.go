package wsapp

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// Store 维护 project、聊天页和消息的内存状态。
type Store struct {
	mu              sync.RWMutex
	projects        map[string]Project
	chats           map[string]Chat
	nextChatOrdinal map[string]int
	agentProfiles   []AgentProfile
	lastAgent       LastAgentSelection
	persister       StorePersister
}

// StorePersister 表示 Store 状态持久化接口。
type StorePersister interface {
	Save(state PersistedStoreState) error // Save 使用 state 参数保存完整 Store 状态。
}

// storeState 表示 Store 可提交的完整内存状态。
type storeState struct {
	projects        map[string]Project
	chats           map[string]Chat
	nextChatOrdinal map[string]int
	agentProfiles   []AgentProfile
	lastAgent       LastAgentSelection
}

// NewStore 创建使用默认 agent 选项的内存状态存储。
func NewStore() *Store {
	return NewStoreWithAgentProfiles(AgentProfiles(AgentOptionsConfig{}))
}

// NewStoreWithAgentProviders 使用 agentProviders 参数创建内存状态存储。
func NewStoreWithAgentProviders(agentProviders []AgentProviderOption) *Store {
	return NewStoreWithAgentProfiles(AgentProfilesFromProviderOptions(agentProviders))
}

// NewStoreWithAgentProfiles 使用 agentProfiles 参数创建内存状态存储。
func NewStoreWithAgentProfiles(agentProfiles []AgentProfile) *Store {
	if len(agentProfiles) == 0 {
		agentProfiles = AgentProfiles(AgentOptionsConfig{})
	}
	return newStoreFromState(storeState{
		projects:        make(map[string]Project),
		chats:           make(map[string]Chat),
		nextChatOrdinal: make(map[string]int),
		agentProfiles:   cloneAgentProfiles(agentProfiles),
		lastAgent:       defaultLastAgentSelection(AgentProviderOptionsFromProfiles(agentProfiles)),
	}, nil)
}

// newStoreFromState 使用 state 和 persister 参数创建 Store。
func newStoreFromState(state storeState, persister StorePersister) *Store {
	normalizeStoreState(&state)
	return &Store{
		projects:        state.projects,
		chats:           state.chats,
		nextChatOrdinal: state.nextChatOrdinal,
		agentProfiles:   state.agentProfiles,
		lastAgent:       state.lastAgent,
		persister:       persister,
	}
}

// AgentProviders 返回当前 Store 中可用的 agent 选项。
func (s *Store) AgentProviders() []AgentProviderOption {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return AgentProviderOptionsFromProfiles(s.agentProfiles)
}

// AgentProfiles 返回当前 Store 中可用的 Profile 配置。
func (s *Store) AgentProfiles() []AgentProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAgentProfiles(s.agentProfiles)
}

// commit 使用 mutate 参数生成新状态，先持久化成功后再替换内存状态。
func (s *Store) commit(mutate func(state *storeState) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.cloneStateLocked()
	if err := mutate(&state); err != nil {
		if errors.Is(err, errStoreUnchanged) {
			return errStoreUnchanged
		}
		return err
	}
	normalizeStoreState(&state)
	if s.persister != nil {
		if err := s.persister.Save(persistedStateFromStoreState(state)); err != nil {
			return err
		}
	}
	s.applyStateLocked(state)
	return nil
}

// cloneStateLocked 返回当前 Store 状态副本，调用方必须持有锁。
func (s *Store) cloneStateLocked() storeState {
	projects := make(map[string]Project, len(s.projects))
	for id, project := range s.projects {
		projects[id] = project
	}
	chats := make(map[string]Chat, len(s.chats))
	for id, chat := range s.chats {
		chats[id] = cloneChat(chat)
	}
	nextChatOrdinal := make(map[string]int, len(s.nextChatOrdinal))
	for id, ordinal := range s.nextChatOrdinal {
		nextChatOrdinal[id] = ordinal
	}
	return storeState{
		projects:        projects,
		chats:           chats,
		nextChatOrdinal: nextChatOrdinal,
		agentProfiles:   cloneAgentProfiles(s.agentProfiles),
		lastAgent:       s.lastAgent,
	}
}

// applyStateLocked 使用 state 参数替换当前 Store 状态，调用方必须持有锁。
func (s *Store) applyStateLocked(state storeState) {
	s.projects = state.projects
	s.chats = state.chats
	s.nextChatOrdinal = state.nextChatOrdinal
	s.agentProfiles = state.agentProfiles
	s.lastAgent = state.lastAgent
}

// normalizeStoreState 使用 state 参数补齐 Store 状态默认值。
func normalizeStoreState(state *storeState) {
	if state.projects == nil {
		state.projects = make(map[string]Project)
	}
	if state.chats == nil {
		state.chats = make(map[string]Chat)
	}
	if state.nextChatOrdinal == nil {
		state.nextChatOrdinal = make(map[string]int)
	}
	if state.agentProfiles == nil {
		state.agentProfiles = AgentProfiles(AgentOptionsConfig{})
	} else {
		state.agentProfiles = normalizeAgentProfiles(state.agentProfiles)
	}
	agentProviders := AgentProviderOptionsFromProfiles(state.agentProfiles)
	if strings.TrimSpace(state.lastAgent.Provider) == "" ||
		strings.TrimSpace(state.lastAgent.Model) == "" ||
		!agentSelectionExists(state.lastAgent.Provider, state.lastAgent.Model, agentProviders) {
		state.lastAgent = defaultLastAgentSelection(agentProviders)
	}
	for chatID, chat := range state.chats {
		if chat.AgentLocked {
			if chat.AgentProfile.ID == "" {
				if profile, ok := AgentProfileByID(state.agentProfiles, chat.AgentProvider); ok {
					chat.AgentProfile = profile
					state.chats[chatID] = chat
				}
			}
			continue
		}
		if strings.TrimSpace(chat.AgentProvider) != "" &&
			strings.TrimSpace(chat.AgentModel) != "" &&
			agentSelectionExists(chat.AgentProvider, chat.AgentModel, agentProviders) {
			continue
		}
		defaultAgent := defaultLastAgentSelection(agentProviders)
		chat.AgentProvider = defaultAgent.Provider
		chat.AgentModel = defaultAgent.Model
		chat.AgentReasoning = defaultAgent.Reasoning
		chat.AgentProfile = AgentProfile{}
		state.chats[chatID] = chat
	}
	for projectID := range state.projects {
		if _, ok := state.nextChatOrdinal[projectID]; !ok {
			state.nextChatOrdinal[projectID] = countProjectChats(state.chats, projectID)
		}
	}
}

// countProjectChats 使用 chats 和 projectID 参数统计 project 下的聊天页数量。
func countProjectChats(chats map[string]Chat, projectID string) int {
	count := 0
	for _, chat := range chats {
		if chat.ProjectID == projectID {
			count++
		}
	}
	return count
}

// agentSelectionExists 使用 provider、model 和 options 参数判断 agent 选择是否存在。
func agentSelectionExists(provider string, model string, options []AgentProviderOption) bool {
	for _, option := range options {
		if option.ID != strings.TrimSpace(provider) {
			continue
		}
		for _, item := range option.Models {
			if item.ID == strings.TrimSpace(model) {
				return true
			}
		}
	}
	return false
}

// profileByIDInState 使用 state 和 profileID 参数查找 Profile 及其索引。
func profileByIDInState(state *storeState, profileID string) (AgentProfile, int, bool) {
	normalizedID := strings.TrimSpace(profileID)
	for index, profile := range state.agentProfiles {
		if profile.ID == normalizedID {
			return cloneAgentProfile(profile), index, true
		}
	}
	return AgentProfile{}, -1, false
}

// applyProfileToChats 使用 state 和 profile 参数同步已选择该 Profile 的聊天页。
func applyProfileToChats(state *storeState, profile AgentProfile) {
	options := AgentProviderOptionsFromProfiles([]AgentProfile{profile})
	for chatID, chat := range state.chats {
		if chat.AgentProvider != profile.ID {
			continue
		}
		if !agentSelectionExists(chat.AgentProvider, chat.AgentModel, options) {
			chat.AgentModel = DefaultAgentModel(profile.ID, options)
			chat.AgentReasoning = DefaultAgentReasoning(profile.ID, chat.AgentModel, options)
		} else {
			_, _, reasoning, err := NormalizeAgentSelection(chat.AgentProvider, chat.AgentModel, chat.AgentReasoning, options)
			if err == nil {
				chat.AgentReasoning = reasoning
			}
		}
		if chat.AgentLocked {
			chat.AgentProfile = profile
		}
		chat.ContextWindow = ContextWindowUsage{}
		chat.UpdatedAt = time.Now()
		state.chats[chatID] = chat
	}
	if state.lastAgent.Provider == profile.ID && !agentSelectionExists(state.lastAgent.Provider, state.lastAgent.Model, options) {
		state.lastAgent = defaultLastAgentSelection(options)
	}
}

// hasDefaultAgentModel 使用 models 参数判断是否已有默认模型。
func hasDefaultAgentModel(models []AgentModelOption) bool {
	for _, model := range models {
		if model.Default {
			return true
		}
	}
	return false
}
