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

	mockOptions := AgentProviderOptions(AgentOptionsConfig{EnableMockAgent: true})
	if !agentOptionsContainProvider(mockOptions, AgentProviderMockClaudeCode) || !agentOptionsContainProvider(mockOptions, AgentProviderMockCodex) {
		t.Fatalf("启用 Mock Agent 后应包含 mock provider: %#v", mockOptions)
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
