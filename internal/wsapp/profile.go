package wsapp

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	// AgentProfileTypeClaudeCode 表示 Claude Code Profile 类型。
	AgentProfileTypeClaudeCode = "claude_code"
	// AgentProfileTypeCodex 表示 Codex Profile 类型。
	AgentProfileTypeCodex = "codex"
)

const (
	// AgentBuiltinProfileKindClaudeCode 表示新增内置 Claude Code Profile。
	AgentBuiltinProfileKindClaudeCode = "claude_code"
	// AgentBuiltinProfileKindCodex 表示新增内置 Codex Profile。
	AgentBuiltinProfileKindCodex = "codex"
	// AgentBuiltinProfileKindMockClaudeCode 表示新增内置 Mock Claude Code Profile。
	AgentBuiltinProfileKindMockClaudeCode = "mock_claude_code"
	// AgentBuiltinProfileKindMockCodex 表示新增内置 Mock Codex Profile。
	AgentBuiltinProfileKindMockCodex = "mock_codex"
)

// BackendEnvSnapshot 返回后端当前启动环境变量的稳定排序快照。
func BackendEnvSnapshot() []BackendEnvVar {
	return envListSnapshot(os.Environ())
}

// AgentProfileByID 使用 profiles 和 id 参数查找 Profile。
func AgentProfileByID(profiles []AgentProfile, id string) (AgentProfile, bool) {
	normalizedID := strings.TrimSpace(id)
	for _, profile := range profiles {
		if profile.ID == normalizedID {
			return cloneAgentProfile(profile), true
		}
	}
	return AgentProfile{}, false
}

// AgentProfilesFromProviderOptions 使用 options 参数兼容旧 provider 持久化数据。
func AgentProfilesFromProviderOptions(options []AgentProviderOption) []AgentProfile {
	profiles := make([]AgentProfile, 0, len(options))
	for _, option := range options {
		profileType := AgentProfileTypeClaudeCode
		command := "claude"
		if strings.Contains(option.ID, "codex") {
			profileType = AgentProfileTypeCodex
			command = "codex"
		}
		profile := AgentProfile{
			ID:      strings.TrimSpace(option.ID),
			Label:   strings.TrimSpace(option.Label),
			Type:    profileType,
			Command: command,
			Models:  cloneAgentModels(option.Models),
			Builtin: option.ID == AgentProviderClaudeCode || option.ID == AgentProviderCodex || option.ID == AgentProviderMockClaudeCode || option.ID == AgentProviderMockCodex,
			Mock:    option.ID == AgentProviderMockClaudeCode || option.ID == AgentProviderMockCodex,
		}
		if profile.Type == AgentProfileTypeClaudeCode {
			profile.Env = claudeCodeBaseEnv()
		}
		normalized, err := normalizeAgentProfile(profile)
		if err == nil {
			profiles = append(profiles, normalized)
		}
	}
	return profiles
}

// BuiltinAgentProfile 使用 kind、config 和 existing 参数创建一个新的内置 Profile。
func BuiltinAgentProfile(kind string, config AgentOptionsConfig, existing []AgentProfile) (AgentProfile, error) {
	profiles := AgentProfiles(config)
	var targetID string
	switch strings.TrimSpace(kind) {
	case AgentBuiltinProfileKindClaudeCode:
		targetID = AgentProviderClaudeCode
	case AgentBuiltinProfileKindCodex:
		targetID = AgentProviderCodex
	case AgentBuiltinProfileKindMockClaudeCode:
		targetID = AgentProviderMockClaudeCode
	case AgentBuiltinProfileKindMockCodex:
		targetID = AgentProviderMockCodex
	default:
		return AgentProfile{}, fmt.Errorf("%w: 不支持的内置 Profile: %s", ErrInvalidInput, kind)
	}
	for _, profile := range profiles {
		if profile.ID != targetID {
			continue
		}
		profile.ID = uniqueProfileID(profile.ID, existing)
		return normalizeAgentProfile(profile)
	}
	return AgentProfile{}, fmt.Errorf("%w: 当前启动配置未启用该内置 Profile: %s", ErrInvalidInput, kind)
}

// hasMockProfile 使用 profiles 参数判断当前启动配置是否包含 Mock Profile。
func hasMockProfile(profiles []AgentProfile) bool {
	for _, profile := range profiles {
		if profile.Mock {
			return true
		}
	}
	return false
}

// EffectiveAgentEnv 使用 backendEnv 和 profile 参数生成实际启动 agent 的环境变量。
func EffectiveAgentEnv(backendEnv []BackendEnvVar, profile AgentProfile) []BackendEnvVar {
	env := make(map[string]string, len(backendEnv)+len(profile.Env))
	for _, item := range backendEnv {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		env[name] = item.Value
	}
	for _, item := range profile.Env {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if item.Unset {
			delete(env, name)
			continue
		}
		env[name] = item.Value
	}
	return envMapSnapshot(env)
}

// EffectiveAgentEnvList 使用 backendEnv 和 profile 参数生成 exec.Cmd 可使用的环境变量列表。
func EffectiveAgentEnvList(backendEnv []BackendEnvVar, profile AgentProfile) []string {
	snapshot := EffectiveAgentEnv(backendEnv, profile)
	result := make([]string, 0, len(snapshot))
	for _, item := range snapshot {
		result = append(result, item.Name+"="+item.Value)
	}
	return result
}

// normalizeAgentProfile 使用 profile 参数校验并补全 Profile。
func normalizeAgentProfile(profile AgentProfile) (AgentProfile, error) {
	profile.ID = strings.TrimSpace(profile.ID)
	profile.Label = strings.TrimSpace(profile.Label)
	profile.Type = strings.TrimSpace(profile.Type)
	profile.Command = strings.TrimSpace(profile.Command)
	if profile.ID == "" {
		return AgentProfile{}, fmt.Errorf("%w: Profile ID 不能为空", ErrInvalidInput)
	}
	if profile.Label == "" {
		profile.Label = profile.ID
	}
	switch profile.Type {
	case AgentProfileTypeClaudeCode:
		if profile.Command == "" {
			profile.Command = "claude"
		}
	case AgentProfileTypeCodex:
		if profile.Command == "" {
			profile.Command = "codex"
		}
	default:
		return AgentProfile{}, fmt.Errorf("%w: 不支持的 Profile 类型: %s", ErrInvalidInput, profile.Type)
	}
	profile.Args = normalizeAgentProfileArgs(profile.Args)
	env, err := normalizeAgentProfileEnv(profile.Env)
	if err != nil {
		return AgentProfile{}, err
	}
	models, err := normalizeAgentProfileModels(profile.Models)
	if err != nil {
		return AgentProfile{}, err
	}
	profile.Env = env
	profile.Models = models
	return profile, nil
}

// normalizeAgentProfiles 使用 profiles 参数校验并补全 Profile 列表。
func normalizeAgentProfiles(profiles []AgentProfile) []AgentProfile {
	result := make([]AgentProfile, 0, len(profiles))
	seen := map[string]bool{}
	for _, profile := range profiles {
		normalized, err := normalizeAgentProfile(profile)
		if err != nil || seen[normalized.ID] {
			continue
		}
		seen[normalized.ID] = true
		result = append(result, normalized)
	}
	return result
}

// normalizeAgentProfileArgs 使用 args 参数返回去除空项后的固定参数。
func normalizeAgentProfileArgs(args []string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// normalizeAgentProfileEnv 使用 env 参数校验并去重环境变量配置。
func normalizeAgentProfileEnv(env []AgentEnvVar) ([]AgentEnvVar, error) {
	result := make([]AgentEnvVar, 0, len(env))
	indexByName := map[string]int{}
	for _, item := range env {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if strings.ContainsAny(name, "=\x00") {
			return nil, fmt.Errorf("%w: 环境变量名不合法: %s", ErrInvalidInput, name)
		}
		normalized := AgentEnvVar{Name: name, Value: item.Value, Unset: item.Unset}
		if item.Unset {
			normalized.Value = ""
		}
		if index, ok := indexByName[name]; ok {
			result[index] = normalized
			continue
		}
		indexByName[name] = len(result)
		result = append(result, normalized)
	}
	return result, nil
}

// normalizeAgentProfileModels 使用 models 参数校验并补全模型列表。
func normalizeAgentProfileModels(models []AgentModelOption) ([]AgentModelOption, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("%w: Profile 至少需要一个模型", ErrInvalidInput)
	}
	result := make([]AgentModelOption, 0, len(models))
	seen := map[string]bool{}
	defaultIndex := -1
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		model.Label = strings.TrimSpace(model.Label)
		if model.ID == "" {
			return nil, fmt.Errorf("%w: 模型标识不能为空", ErrInvalidInput)
		}
		if seen[model.ID] {
			return nil, fmt.Errorf("%w: 模型已存在: %s", ErrInvalidInput, model.ID)
		}
		if model.Label == "" {
			model.Label = model.ID
		}
		model.ReasoningLevels = normalizeReasoningLevels(model.ReasoningLevels)
		if model.Default && defaultIndex == -1 {
			defaultIndex = len(result)
		}
		model.Default = false
		seen[model.ID] = true
		result = append(result, model)
	}
	if defaultIndex < 0 {
		defaultIndex = 0
	}
	result[defaultIndex].Default = true
	return result, nil
}

// normalizeReasoningLevels 使用 levels 参数校验并补全推理级别。
func normalizeReasoningLevels(levels []AgentReasoningOption) []AgentReasoningOption {
	result := make([]AgentReasoningOption, 0, len(levels))
	seen := map[string]bool{}
	defaultIndex := -1
	for _, level := range levels {
		level.ID = strings.TrimSpace(level.ID)
		level.Label = strings.TrimSpace(level.Label)
		level.Description = strings.TrimSpace(level.Description)
		if level.ID == "" || seen[level.ID] {
			continue
		}
		if level.Label == "" {
			level.Label = level.ID
		}
		if level.Default && defaultIndex == -1 {
			defaultIndex = len(result)
		}
		level.Default = false
		seen[level.ID] = true
		result = append(result, level)
	}
	if defaultIndex >= 0 && len(result) > 0 {
		result[defaultIndex].Default = true
	}
	return result
}

// claudeCodeBaseEnv 返回 Claude Code Profile 默认环境变量配置。
func claudeCodeBaseEnv() []AgentEnvVar {
	env := []AgentEnvVar{
		{Name: "CLAUDECODE", Unset: true},
		{Name: "CLAUDE_CODE_ENTRYPOINT", Unset: true},
		{Name: "CLAUDE_CODE_SSE_PORT", Unset: true},
		{Name: "CLAUDE_AGENT_SDK_VERSION", Unset: true},
		{Name: "CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING", Unset: true},
		{Name: "IS_SANDBOX", Value: "1"},
		{Name: "DISABLE_AUTOUPDATER", Value: "1"},
	}
	return env
}

// mockClaudeEnv 使用 config 参数返回 Mock Claude Code Profile 环境变量配置。
func mockClaudeEnv(config AgentOptionsConfig) []AgentEnvVar {
	env := claudeCodeBaseEnv()
	if baseURL := strings.TrimRight(strings.TrimSpace(config.MockAnthropicBaseURL), "/"); baseURL != "" {
		env = append(env,
			AgentEnvVar{Name: "ANTHROPIC_BASE_URL", Value: baseURL},
			AgentEnvVar{Name: "CLAUDE_CODE_API_BASE_URL", Value: baseURL},
		)
	}
	if apiKey := strings.TrimSpace(config.MockAnthropicAPIKey); apiKey != "" {
		env = append(env, AgentEnvVar{Name: "ANTHROPIC_API_KEY", Value: apiKey})
	}
	env = append(env, AgentEnvVar{Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1"})
	return env
}

// mockCodexEnv 使用 config 参数返回 Mock Codex Profile 环境变量配置。
func mockCodexEnv(config AgentOptionsConfig) []AgentEnvVar {
	env := make([]AgentEnvVar, 0, 4)
	if baseURL := strings.TrimRight(strings.TrimSpace(config.MockOpenAIBaseURL), "/"); baseURL != "" {
		env = append(env, AgentEnvVar{Name: "OPENAI_BASE_URL", Value: baseURL})
	}
	if apiKey := strings.TrimSpace(config.MockOpenAIAPIKey); apiKey != "" {
		env = append(env, AgentEnvVar{Name: "OPENAI_API_KEY", Value: apiKey})
	}
	if baseURL := strings.TrimRight(strings.TrimSpace(config.MockAnthropicBaseURL), "/"); baseURL != "" {
		env = append(env, AgentEnvVar{Name: "ANTHROPIC_BASE_URL", Value: baseURL})
	}
	if apiKey := strings.TrimSpace(config.MockAnthropicAPIKey); apiKey != "" {
		env = append(env, AgentEnvVar{Name: "ANTHROPIC_API_KEY", Value: apiKey})
	}
	return env
}

// mockCodexConfigArgs 使用 baseURL 参数返回 Mock Codex 固定配置参数。
func mockCodexConfigArgs(baseURL string) []string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil
	}
	providerConfig := fmt.Sprintf(
		`{name="AgentHub Mock OpenAI", base_url=%q, env_key="OPENAI_API_KEY", wire_api="responses"}`,
		trimmed,
	)
	return []string{
		"--ignore-user-config",
		"-c", `model_provider="agenthub_mock"`,
		"-c", "model_providers.agenthub_mock=" + providerConfig,
	}
}

// uniqueProfileID 使用 baseID 和 existing 参数生成不冲突的 Profile ID。
func uniqueProfileID(baseID string, existing []AgentProfile) string {
	used := map[string]bool{}
	for _, profile := range existing {
		used[profile.ID] = true
	}
	if !used[baseID] {
		return baseID
	}
	for index := 2; ; index++ {
		id := fmt.Sprintf("%s-%d", baseID, index)
		if !used[id] {
			return id
		}
	}
}

// envListSnapshot 使用 env 参数生成稳定排序的环境变量快照。
func envListSnapshot(env []string) []BackendEnvVar {
	result := make([]BackendEnvVar, 0, len(env))
	for _, pair := range env {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		result = append(result, BackendEnvVar{Name: key, Value: value})
	}
	sort.Slice(result, func(i int, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// envMapSnapshot 使用 env 参数生成稳定排序的环境变量快照。
func envMapSnapshot(env map[string]string) []BackendEnvVar {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]BackendEnvVar, 0, len(keys))
	for _, key := range keys {
		result = append(result, BackendEnvVar{Name: key, Value: env[key]})
	}
	return result
}

// cloneAgentProfiles 使用 profiles 参数创建不会共享切片的副本。
func cloneAgentProfiles(profiles []AgentProfile) []AgentProfile {
	result := make([]AgentProfile, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, cloneAgentProfile(profile))
	}
	return result
}

// cloneAgentProfile 使用 profile 参数创建不会共享切片的副本。
func cloneAgentProfile(profile AgentProfile) AgentProfile {
	profile.Args = append([]string(nil), profile.Args...)
	profile.Env = append([]AgentEnvVar(nil), profile.Env...)
	profile.Models = cloneAgentModels(profile.Models)
	return profile
}

// cloneAgentModels 使用 models 参数创建不会共享推理级别切片的副本。
func cloneAgentModels(models []AgentModelOption) []AgentModelOption {
	result := append([]AgentModelOption(nil), models...)
	for index := range result {
		result[index].ReasoningLevels = append([]AgentReasoningOption(nil), result[index].ReasoningLevels...)
	}
	return result
}
