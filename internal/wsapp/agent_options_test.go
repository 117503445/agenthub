package wsapp

import "testing"

// TestAgentProviderOptionsGateMockAgents 验证 Mock Agent 只在显式启用时出现。
func TestAgentProviderOptionsGateMockAgents(t *testing.T) {
	normalOptions := AgentProviderOptions(AgentOptionsConfig{})
	if agentOptionsContainProvider(normalOptions, AgentProviderMockClaudeCode) || agentOptionsContainProvider(normalOptions, AgentProviderMockCodex) {
		t.Fatalf("默认 agent 选项不应包含 mock provider: %#v", normalOptions)
	}
	if !agentOptionsContainProvider(normalOptions, AgentProviderClaudeCode) || !agentOptionsContainProvider(normalOptions, AgentProviderCodex) {
		t.Fatalf("默认 agent 选项应包含真实 provider: %#v", normalOptions)
	}
	if !agentModelLabelsEqualIDs(normalOptions) {
		t.Fatalf("模型展示值应与模型标识一致: %#v", normalOptions)
	}

	mockOptions := AgentProviderOptions(AgentOptionsConfig{EnableMockAgent: true})
	if !agentOptionsContainProvider(mockOptions, AgentProviderMockClaudeCode) || !agentOptionsContainProvider(mockOptions, AgentProviderMockCodex) {
		t.Fatalf("启用 Mock Agent 后应包含 mock provider: %#v", mockOptions)
	}
	if !agentModelLabelsEqualIDs(mockOptions) {
		t.Fatalf("Mock 模型展示值应与模型标识一致: %#v", mockOptions)
	}
}

// TestMockProfilesUseNormalProfileShape 验证 Mock Profile 仅通过 endpoint 环境变量区分。
func TestMockProfilesUseNormalProfileShape(t *testing.T) {
	profiles := AgentProfiles(AgentOptionsConfig{
		EnableMockAgent:      true,
		MockAnthropicBaseURL: "http://127.0.0.1:17375/mock/anthropic",
		MockOpenAIBaseURL:    "http://127.0.0.1:17375/mock/openai/v1",
		MockAnthropicAPIKey:  "mock-key",
		MockOpenAIAPIKey:     "mock-key",
	})
	claude, ok := AgentProfileByID(profiles, AgentProviderMockClaudeCode)
	if !ok {
		t.Fatal("缺少 Mock Claude Code Profile")
	}
	if claude.Type != AgentProfileTypeClaudeCode || len(claude.Args) != 0 {
		t.Fatalf("Mock Claude Code 应与普通 Claude Profile 使用同一启动形态: %#v", claude)
	}
	if !agentEnvContains(claude.Env, "ANTHROPIC_BASE_URL", "http://127.0.0.1:17375/mock/anthropic") {
		t.Fatalf("Mock Claude Code endpoint 环境变量不正确: %#v", claude.Env)
	}
	codex, ok := AgentProfileByID(profiles, AgentProviderMockCodex)
	if !ok {
		t.Fatal("缺少 Mock Codex Profile")
	}
	if codex.Type != AgentProfileTypeCodex || len(codex.Args) != 0 {
		t.Fatalf("Mock Codex 应与普通 Codex Profile 使用同一启动形态: %#v", codex)
	}
	if !agentEnvContains(codex.Env, "OPENAI_BASE_URL", "http://127.0.0.1:17375/mock/openai/v1") {
		t.Fatalf("Mock Codex endpoint 环境变量不正确: %#v", codex.Env)
	}
}

// agentOptionsContainProvider 使用 options 和 provider 参数判断 provider 是否存在。
func agentOptionsContainProvider(options []AgentProviderOption, provider string) bool {
	for _, option := range options {
		if option.ID == provider {
			return true
		}
	}
	return false
}

// agentEnvContains 使用 env、name 和 value 参数判断环境变量是否存在。
func agentEnvContains(env []AgentEnvVar, name string, value string) bool {
	for _, item := range env {
		if item.Name == name && item.Value == value {
			return true
		}
	}
	return false
}

// agentModelLabelsEqualIDs 使用 options 参数判断所有模型展示值是否等于模型标识。
func agentModelLabelsEqualIDs(options []AgentProviderOption) bool {
	for _, option := range options {
		for _, model := range option.Models {
			if model.Label != model.ID {
				return false
			}
		}
	}
	return true
}
