package wsapp

import (
	"context"
	"errors"
	"reflect"
	"sort"
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
	skillsMu        sync.Mutex
	skillsCache     agentSkillCache
	runtimeSkills   []AgentSkillOption
}

// StorePersister 表示 Store 状态持久化接口。
type StorePersister interface {
	// SaveAll 使用 state 参数保存完整 Store 状态。
	SaveAll(state PersistedStoreState) error
	// SaveChanges 使用 change 参数保存 Store 增量变更。
	SaveChanges(change PersistedStoreChange) error
	// Flush 使用 ctx 参数等待已延迟的持久化写入完成。
	Flush(ctx context.Context) error
}

// storeState 表示 Store 可提交的完整内存状态。
type storeState struct {
	projects        map[string]Project
	chats           map[string]Chat
	nextChatOrdinal map[string]int
	agentProfiles   []AgentProfile
	lastAgent       LastAgentSelection
}

// storeCommitOptions 表示一次 Store 提交的持久化策略。
type storeCommitOptions struct {
	deferChatDetails bool // deferChatDetails 表示聊天详情是否允许延迟落盘。
}

// agentSkillCache 表示 Store 中缓存的 skills 扫描结果。
type agentSkillCache struct {
	key    string             // key 表示 project 搜索路径签名。
	skills []AgentSkillOption // skills 表示缓存的 skill 列表。
	paths  []string           // paths 表示缓存的搜索路径列表。
	loaded bool               // loaded 表示缓存是否已有有效结果。
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
	return s.commitWithOptions(storeCommitOptions{}, mutate)
}

// commitDeferredChatDetails 使用 mutate 参数生成新状态，并允许聊天详情延迟落盘。
func (s *Store) commitDeferredChatDetails(mutate func(state *storeState) error) error {
	return s.commitWithOptions(storeCommitOptions{deferChatDetails: true}, mutate)
}

// commitWithOptions 使用 options 和 mutate 参数提交 Store 状态变更。
func (s *Store) commitWithOptions(options storeCommitOptions, mutate func(state *storeState) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := s.cloneStateLocked()
	state := s.cloneStateLocked()
	if err := mutate(&state); err != nil {
		if errors.Is(err, errStoreUnchanged) {
			return errStoreUnchanged
		}
		return err
	}
	normalizeStoreState(&state)
	if s.persister != nil {
		change := persistedStoreChangeFromStates(before, state, options.deferChatDetails)
		if err := s.persister.SaveChanges(change); err != nil {
			return err
		}
	}
	s.applyStateLocked(state)
	return nil
}

// Flush 使用 ctx 参数等待 Store 延迟持久化写入完成。
func (s *Store) Flush(ctx context.Context) error {
	if s.persister == nil {
		return nil
	}
	return s.persister.Flush(ctx)
}

// persistedStoreChangeFromStates 使用 before、after 和 deferChatDetails 参数生成持久化变更集。
func persistedStoreChangeFromStates(before storeState, after storeState, deferChatDetails bool) PersistedStoreChange {
	beforePersisted := persistedStateFromStoreState(before)
	afterPersisted := persistedStateFromStoreState(after)
	beforeMeta := beforePersisted
	beforeMeta.ChatDetails = nil
	afterMeta := afterPersisted
	afterMeta.ChatDetails = nil

	beforeDetails := persistedChatDetailsByID(beforePersisted.ChatDetails)
	afterDetails := persistedChatDetailsByID(afterPersisted.ChatDetails)
	dirtyChatIDs := make([]string, 0)
	deletedChatIDs := make([]string, 0)
	for chatID, afterDetail := range afterDetails {
		beforeDetail, ok := beforeDetails[chatID]
		if !ok || !reflect.DeepEqual(beforeDetail, afterDetail) {
			dirtyChatIDs = append(dirtyChatIDs, chatID)
		}
	}
	for chatID := range beforeDetails {
		if _, ok := afterDetails[chatID]; !ok {
			deletedChatIDs = append(deletedChatIDs, chatID)
		}
	}
	sort.Strings(dirtyChatIDs)
	sort.Strings(deletedChatIDs)
	return PersistedStoreChange{
		State:            afterPersisted,
		MetaDirty:        !reflect.DeepEqual(beforeMeta, afterMeta),
		DirtyChatIDs:     dirtyChatIDs,
		DeletedChatIDs:   deletedChatIDs,
		DeferChatDetails: deferChatDetails,
	}
}

// persistedChatDetailsByID 使用 details 参数按聊天页标识索引详情。
func persistedChatDetailsByID(details []PersistedChatDetail) map[string]PersistedChatDetail {
	result := make(map[string]PersistedChatDetail, len(details))
	for _, detail := range details {
		result[detail.ChatID] = detail
	}
	return result
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
			} else {
				chat.AgentReasoning = DefaultAgentReasoning(profile.ID, chat.AgentModel, options)
			}
		}
		if chat.AgentLocked {
			chat.AgentProfile = profile
		}
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
