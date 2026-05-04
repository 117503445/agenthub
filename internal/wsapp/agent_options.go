package wsapp

import (
	"fmt"
	"strings"
)

// AgentOptionsConfig 表示 agent 选项生成配置。
type AgentOptionsConfig struct {
	ClaudeDefaultModel   string // ClaudeDefaultModel 表示 Claude 默认模型。
	CodexDefaultEffort   string // CodexDefaultEffort 表示 Codex 默认推理级别。
	EnableMockAgent      bool   // EnableMockAgent 表示是否展示 Mock Agent 选项。
	ClaudeCommand        string // ClaudeCommand 表示 Claude Code 启动命令。
	CodexCommand         string // CodexCommand 表示 Codex 启动命令。
	MockClaudeCommand    string // MockClaudeCommand 表示 Mock Claude Code 启动命令。
	MockCodexCommand     string // MockCodexCommand 表示 Mock Codex 启动命令。
	MockAnthropicBaseURL string // MockAnthropicBaseURL 表示 Mock Anthropic 兼容接口地址。
	MockAnthropicAPIKey  string // MockAnthropicAPIKey 表示 Mock Anthropic API Key。
	MockOpenAIBaseURL    string // MockOpenAIBaseURL 表示 Mock OpenAI 兼容接口地址。
	MockOpenAIAPIKey     string // MockOpenAIAPIKey 表示 Mock OpenAI API Key。
}

// AgentReasoningOption 表示一个可选推理级别。
type AgentReasoningOption struct {
	ID          string `json:"id"`          // ID 表示传递给 agent CLI 的推理级别标识。
	Label       string `json:"label"`       // Label 表示界面展示名称。
	Description string `json:"description"` // Description 表示推理级别说明。
	Default     bool   `json:"default"`     // Default 表示是否为默认推理级别。
}

// AgentModelOption 表示一个可选 agent 模型。
type AgentModelOption struct {
	ID              string                 `json:"id"`                        // ID 表示传递给 agent CLI 的模型标识。
	Label           string                 `json:"label"`                     // Label 表示界面展示名称。
	Default         bool                   `json:"default"`                   // Default 表示是否为该 provider 默认模型。
	ReasoningLevels []AgentReasoningOption `json:"reasoningLevels,omitempty"` // ReasoningLevels 表示模型支持的推理级别。
}

// AgentProviderOption 表示一个可选 agent provider。
type AgentProviderOption struct {
	ID     string             `json:"id"`     // ID 表示 provider 标识。
	Label  string             `json:"label"`  // Label 表示界面展示名称。
	Models []AgentModelOption `json:"models"` // Models 表示 provider 可选模型。
}

// AgentEnvVar 表示 Profile 叠加到后端启动环境变量上的环境变量配置。
type AgentEnvVar struct {
	Name  string `json:"name"`            // Name 表示环境变量名。
	Value string `json:"value,omitempty"` // Value 表示环境变量值。
	Unset bool   `json:"unset"`           // Unset 表示删除后端同名环境变量。
}

// AgentProfile 表示一套可启动 agent 的运行配置。
type AgentProfile struct {
	ID      string             `json:"id"`      // ID 表示 Profile 唯一标识。
	Label   string             `json:"label"`   // Label 表示界面展示名称。
	Type    string             `json:"type"`    // Type 表示 Profile 类型。
	Command string             `json:"command"` // Command 表示启动命令。
	Args    []string           `json:"args"`    // Args 表示固定命令参数。
	Env     []AgentEnvVar      `json:"env"`     // Env 表示 Profile 环境变量配置。
	Models  []AgentModelOption `json:"models"`  // Models 表示聊天中可动态切换的模型列表。
	Builtin bool               `json:"builtin"` // Builtin 表示是否由系统生成的内置 Profile。
	Mock    bool               `json:"mock"`    // Mock 表示是否为 Mock Agent Profile。
}

// BackendEnvVar 表示后端启动时捕获的一项环境变量。
type BackendEnvVar struct {
	Name  string `json:"name"`  // Name 表示环境变量名。
	Value string `json:"value"` // Value 表示环境变量完整值。
}

// AgentProfiles 使用 config 参数返回启动时自动探测生成的内置 Profile。
func AgentProfiles(config AgentOptionsConfig) []AgentProfile {
	claudeDefaultModel := strings.TrimSpace(config.ClaudeDefaultModel)
	if claudeDefaultModel == "" {
		claudeDefaultModel = "sonnet"
	}
	codexDefaultEffort := strings.TrimSpace(config.CodexDefaultEffort)
	if codexDefaultEffort == "" {
		codexDefaultEffort = "xhigh"
	}
	claudeModels := withDefaultModel([]AgentModelOption{
		{ID: "sonnet", Label: "Sonnet"},
		{ID: "opus", Label: "Opus"},
		{ID: "haiku", Label: "Haiku"},
	}, claudeDefaultModel)
	codexReasoningLevels := withDefaultReasoning([]AgentReasoningOption{
		{ID: "low", Label: "Low", Description: "快速响应，使用较轻推理。"},
		{ID: "medium", Label: "Medium", Description: "默认级别，平衡速度和推理深度。"},
		{ID: "high", Label: "High", Description: "更深入推理，适合复杂问题。"},
		{ID: "xhigh", Label: "Extra high", Description: "最深推理，适合复杂实现和排障。"},
	}, codexDefaultEffort)
	claudeCommand := strings.TrimSpace(config.ClaudeCommand)
	if claudeCommand == "" {
		claudeCommand = "claude"
	}
	codexCommand := strings.TrimSpace(config.CodexCommand)
	if codexCommand == "" {
		codexCommand = "codex"
	}
	mockClaudeCommand := strings.TrimSpace(config.MockClaudeCommand)
	if mockClaudeCommand == "" {
		mockClaudeCommand = claudeCommand
	}
	mockCodexCommand := strings.TrimSpace(config.MockCodexCommand)
	if mockCodexCommand == "" {
		mockCodexCommand = codexCommand
	}
	profiles := []AgentProfile{
		{
			ID:      AgentProviderClaudeCode,
			Label:   "Claude Code",
			Type:    AgentProfileTypeClaudeCode,
			Command: claudeCommand,
			Env:     claudeCodeBaseEnv(),
			Models:  claudeModels,
			Builtin: true,
		},
		{
			ID:      AgentProviderCodex,
			Label:   "Codex",
			Type:    AgentProfileTypeCodex,
			Command: codexCommand,
			Models: []AgentModelOption{
				{ID: "gpt-5.5", Label: "GPT-5.5", Default: true, ReasoningLevels: codexReasoningLevels},
				{ID: "gpt-5.4-mini", Label: "GPT-5.4 Mini"},
				{ID: "gpt-5.4", Label: "GPT-5.4"},
				{ID: "gpt-5.3-codex", Label: "GPT-5.3 Codex"},
			},
			Builtin: true,
		},
	}
	if !config.EnableMockAgent {
		return profiles
	}
	return append(profiles,
		AgentProfile{
			ID:      AgentProviderMockClaudeCode,
			Label:   "Mock Claude Code",
			Type:    AgentProfileTypeClaudeCode,
			Command: mockClaudeCommand,
			Args:    []string{"--bare", "--setting-sources", "local"},
			Env:     mockClaudeEnv(config),
			Models: []AgentModelOption{
				{ID: "mock-claude-sonnet", Label: "Mock Claude Sonnet", Default: true},
				{ID: "mock-claude-opus", Label: "Mock Claude Opus"},
			},
			Builtin: true,
			Mock:    true,
		},
		AgentProfile{
			ID:      AgentProviderMockCodex,
			Label:   "Mock Codex",
			Type:    AgentProfileTypeCodex,
			Command: mockCodexCommand,
			Args:    mockCodexConfigArgs(config.MockOpenAIBaseURL),
			Env:     mockCodexEnv(config),
			Models: []AgentModelOption{
				{ID: "mock-codex-gpt-5.5", Label: "Mock Codex GPT-5.5", Default: true, ReasoningLevels: codexReasoningLevels},
				{ID: "mock-codex-fast", Label: "Mock Codex Fast"},
			},
			Builtin: true,
			Mock:    true,
		},
	)
}

// AgentProviderOptions 使用 config 参数返回前端可展示的 agent provider 和模型列表。
func AgentProviderOptions(config AgentOptionsConfig) []AgentProviderOption {
	return AgentProviderOptionsFromProfiles(AgentProfiles(config))
}

// AgentProviderOptionsFromProfiles 使用 profiles 参数生成兼容前端聊天选择框的 provider 列表。
func AgentProviderOptionsFromProfiles(profiles []AgentProfile) []AgentProviderOption {
	options := make([]AgentProviderOption, 0, len(profiles))
	for _, profile := range profiles {
		options = append(options, AgentProviderOption{
			ID:     profile.ID,
			Label:  profile.Label,
			Models: cloneAgentModels(profile.Models),
		})
	}
	return options
}

// DefaultAgentProviderOptions 返回默认 agent provider 和模型列表。
func DefaultAgentProviderOptions() []AgentProviderOption {
	return AgentProviderOptions(AgentOptionsConfig{})
}

// DefaultAgentModel 使用 provider 和 options 参数返回默认模型。
func DefaultAgentModel(provider string, options []AgentProviderOption) string {
	if len(options) == 0 {
		options = DefaultAgentProviderOptions()
	}
	for _, option := range options {
		if option.ID != provider {
			continue
		}
		for _, model := range option.Models {
			if model.Default {
				return model.ID
			}
		}
		if len(option.Models) > 0 {
			return option.Models[0].ID
		}
	}
	defaultProvider := DefaultAgentProvider(options)
	if defaultProvider != provider && defaultProvider != "" {
		return DefaultAgentModel(defaultProvider, options)
	}
	return ""
}

// DefaultAgentReasoning 使用 provider、model 和 options 参数返回默认推理级别。
func DefaultAgentReasoning(provider string, model string, options []AgentProviderOption) string {
	if len(options) == 0 {
		options = DefaultAgentProviderOptions()
	}
	for _, option := range options {
		if option.ID != provider {
			continue
		}
		for _, modelOption := range option.Models {
			if modelOption.ID != model {
				continue
			}
			for _, reasoning := range modelOption.ReasoningLevels {
				if reasoning.Default {
					return reasoning.ID
				}
			}
			if len(modelOption.ReasoningLevels) > 0 {
				return modelOption.ReasoningLevels[0].ID
			}
		}
	}
	return ""
}

// defaultLastAgentSelection 使用 options 参数返回新聊天页的初始 agent 配置。
func defaultLastAgentSelection(options []AgentProviderOption) LastAgentSelection {
	if len(options) == 0 {
		return LastAgentSelection{}
	}
	provider := DefaultAgentProvider(options)
	model := DefaultAgentModel(provider, options)
	return LastAgentSelection{
		Provider:  provider,
		Model:     model,
		Reasoning: DefaultAgentReasoning(provider, model, options),
	}
}

// NormalizeAgentSelection 使用 provider、model、reasoning 和 options 参数校验并补全 agent 选择。
func NormalizeAgentSelection(provider string, model string, reasoning string, options []AgentProviderOption) (string, string, string, error) {
	if len(options) == 0 {
		options = DefaultAgentProviderOptions()
	}
	normalizedProvider := strings.TrimSpace(provider)
	if normalizedProvider == "" {
		normalizedProvider = DefaultAgentProvider(options)
	}
	for _, option := range options {
		if option.ID != normalizedProvider {
			continue
		}
		normalizedModel := strings.TrimSpace(model)
		if normalizedModel == "" {
			normalizedModel = DefaultAgentModel(normalizedProvider, options)
		}
		for _, modelOption := range option.Models {
			if modelOption.ID != normalizedModel {
				continue
			}
			normalizedReasoning := strings.TrimSpace(reasoning)
			if len(modelOption.ReasoningLevels) == 0 {
				return normalizedProvider, normalizedModel, "", nil
			}
			if normalizedReasoning == "" {
				normalizedReasoning = DefaultAgentReasoning(normalizedProvider, normalizedModel, options)
			}
			for _, reasoningOption := range modelOption.ReasoningLevels {
				if reasoningOption.ID == normalizedReasoning {
					return normalizedProvider, normalizedModel, normalizedReasoning, nil
				}
			}
			return "", "", "", fmt.Errorf("%w: 不支持的推理级别: %s", ErrInvalidInput, reasoning)
		}
		return "", "", "", fmt.Errorf("%w: 不支持的模型: %s", ErrInvalidInput, model)
	}
	return "", "", "", fmt.Errorf("%w: 不支持的 agent: %s", ErrInvalidInput, provider)
}

// DefaultAgentProvider 使用 options 参数返回默认 agent provider。
func DefaultAgentProvider(options []AgentProviderOption) string {
	if len(options) == 0 {
		options = DefaultAgentProviderOptions()
	}
	for _, option := range options {
		if strings.TrimSpace(option.ID) != "" {
			return option.ID
		}
	}
	return AgentProviderClaudeCode
}

// withDefaultModel 使用 models 和 defaultModel 参数生成默认模型列表。
func withDefaultModel(models []AgentModelOption, defaultModel string) []AgentModelOption {
	found := false
	for index := range models {
		models[index].Default = models[index].ID == defaultModel
		if models[index].ID == defaultModel {
			found = true
		}
	}
	if found {
		return models
	}
	return append([]AgentModelOption{{ID: defaultModel, Label: defaultModel, Default: true}}, models...)
}

// withDefaultReasoning 使用 levels 和 defaultLevel 参数生成默认推理级别列表。
func withDefaultReasoning(levels []AgentReasoningOption, defaultLevel string) []AgentReasoningOption {
	found := false
	for index := range levels {
		levels[index].Default = levels[index].ID == defaultLevel
		if levels[index].ID == defaultLevel {
			found = true
		}
	}
	if found {
		return levels
	}
	return levels
}
