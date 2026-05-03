package wsapp

import (
	"errors"
	"strings"
	"sync"
)

// Store 维护 project、聊天页和消息的内存状态。
type Store struct {
	mu              sync.RWMutex
	projects        map[string]Project
	chats           map[string]Chat
	nextChatOrdinal map[string]int
	agentProviders  []AgentProviderOption
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
	agentProviders  []AgentProviderOption
	lastAgent       LastAgentSelection
}

// NewStore 创建使用默认 agent 选项的内存状态存储。
func NewStore() *Store {
	return NewStoreWithAgentProviders(DefaultAgentProviderOptions())
}

// NewStoreWithAgentProviders 使用 agentProviders 参数创建内存状态存储。
func NewStoreWithAgentProviders(agentProviders []AgentProviderOption) *Store {
	if len(agentProviders) == 0 {
		agentProviders = DefaultAgentProviderOptions()
	}
	return newStoreFromState(storeState{
		projects:        make(map[string]Project),
		chats:           make(map[string]Chat),
		nextChatOrdinal: make(map[string]int),
		agentProviders:  cloneAgentProviderOptions(agentProviders),
		lastAgent:       defaultLastAgentSelection(agentProviders),
	}, nil)
}

// newStoreFromState 使用 state 和 persister 参数创建 Store。
func newStoreFromState(state storeState, persister StorePersister) *Store {
	normalizeStoreState(&state)
	return &Store{
		projects:        state.projects,
		chats:           state.chats,
		nextChatOrdinal: state.nextChatOrdinal,
		agentProviders:  state.agentProviders,
		lastAgent:       state.lastAgent,
		persister:       persister,
	}
}

// AgentProviders 返回当前 Store 中可用的 agent 选项。
func (s *Store) AgentProviders() []AgentProviderOption {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAgentProviderOptions(s.agentProviders)
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
		agentProviders:  cloneAgentProviderOptions(s.agentProviders),
		lastAgent:       s.lastAgent,
	}
}

// applyStateLocked 使用 state 参数替换当前 Store 状态，调用方必须持有锁。
func (s *Store) applyStateLocked(state storeState) {
	s.projects = state.projects
	s.chats = state.chats
	s.nextChatOrdinal = state.nextChatOrdinal
	s.agentProviders = state.agentProviders
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
	if len(state.agentProviders) == 0 {
		state.agentProviders = DefaultAgentProviderOptions()
	} else {
		state.agentProviders = cloneAgentProviderOptions(state.agentProviders)
	}
	if strings.TrimSpace(state.lastAgent.Provider) == "" || strings.TrimSpace(state.lastAgent.Model) == "" {
		state.lastAgent = defaultLastAgentSelection(state.agentProviders)
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
