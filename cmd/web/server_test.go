package main

import "testing"

// TestResolveAgentConfigUsesBackendMockService 验证 mock agent 固定打到后端 mock 服务。
func TestResolveAgentConfigUsesBackendMockService(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")

	config := resolveAgentConfig("6767")
	if config.AnthropicBaseURL != "" || config.OpenAIBaseURL != "" {
		t.Fatalf("真实 provider 不应该被默认 mock 地址污染: anthropic=%q openai=%q", config.AnthropicBaseURL, config.OpenAIBaseURL)
	}
	if config.MockAnthropicBaseURL != "http://127.0.0.1:6767/mock/anthropic" {
		t.Fatalf("Mock Claude 地址不正确: %q", config.MockAnthropicBaseURL)
	}
	if config.MockOpenAIBaseURL != "http://127.0.0.1:6767/mock/openai/v1" {
		t.Fatalf("Mock Codex 地址不正确: %q", config.MockOpenAIBaseURL)
	}
	if config.MockAnthropicAPIKey == "" || config.MockOpenAIAPIKey == "" {
		t.Fatalf("mock agent 应该带有 mock API key")
	}
}
