package main

import "testing"

// TestResolveAgentConfigUsesBackendMockService 验证 mock agent 固定打到后端 mock 服务。
func TestResolveAgentConfigUsesBackendMockService(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "https://legacy-anthropic.example")
	t.Setenv("ANTHROPIC_API_KEY", "legacy-anthropic-key")
	t.Setenv("ANTHROPIC_MODEL", "legacy-model")
	t.Setenv("OPENAI_BASE_URL", "https://legacy-openai.example/v1")
	t.Setenv("OPENAI_API_KEY", "legacy-openai-key")
	t.Setenv("CODING_CLAUDE_COMMAND", "legacy-claude")
	t.Setenv("CODING_CODEX_COMMAND", "legacy-codex")
	t.Setenv("CODING_MOCK_CLAUDE_COMMAND", "legacy-mock-claude")
	t.Setenv("CODING_MOCK_CODEX_COMMAND", "legacy-mock-codex")

	config := resolveAgentConfig("6767")
	if config.AnthropicBaseURL != "" || config.OpenAIBaseURL != "" {
		t.Fatalf("真实 provider 不应该被默认 mock 地址污染: anthropic=%q openai=%q", config.AnthropicBaseURL, config.OpenAIBaseURL)
	}
	if config.AnthropicAPIKey != "" || config.OpenAIAPIKey != "" {
		t.Fatalf("真实 provider 不应该读取旧 API Key 环境变量")
	}
	if config.AnthropicModel != "sonnet" {
		t.Fatalf("默认 Claude 模型不应读取旧环境变量，当前值: %q", config.AnthropicModel)
	}
	if config.Command != "claude" || config.CodexCommand != "codex" {
		t.Fatalf("真实命令不应读取旧环境变量: claude=%q codex=%q", config.Command, config.CodexCommand)
	}
	if config.MockClaudeCommand != "claude" || config.MockCodexCommand != "codex" {
		t.Fatalf("mock 命令不应读取旧环境变量: claude=%q codex=%q", config.MockClaudeCommand, config.MockCodexCommand)
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
