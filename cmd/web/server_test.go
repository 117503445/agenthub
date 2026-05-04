package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/117503445/agenthub/internal/wsapp"
)

// TestResolveAgentConfigUsesBackendMockService 验证 mock agent 固定打到后端 mock 服务。
func TestResolveAgentConfigUsesBackendMockService(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "https://legacy-anthropic.example")
	t.Setenv("ANTHROPIC_API_KEY", "legacy-anthropic-key")
	t.Setenv("ANTHROPIC_MODEL", "legacy-model")
	t.Setenv("OPENAI_BASE_URL", "https://legacy-openai.example/v1")
	t.Setenv("OPENAI_API_KEY", "legacy-openai-key")
	t.Setenv("LEGACY_AGENTHUB_CLAUDE_COMMAND", "legacy-claude")
	t.Setenv("LEGACY_AGENTHUB_CODEX_COMMAND", "legacy-codex")
	t.Setenv("LEGACY_AGENTHUB_MOCK_CLAUDE_COMMAND", "legacy-mock-claude")
	t.Setenv("LEGACY_AGENTHUB_MOCK_CODEX_COMMAND", "legacy-mock-codex")

	config := resolveAgentConfig(webConfig{Port: "6767"})
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
	if serverTestAgentProvidersContain(config.AgentProviders, wsapp.AgentProviderMockClaudeCode) || serverTestAgentProvidersContain(config.AgentProviders, wsapp.AgentProviderMockCodex) {
		t.Fatalf("未启用 MOCK_AGENT 时不应返回 mock provider: %#v", config.AgentProviders)
	}

	config = resolveAgentConfig(webConfig{Port: "6767", MockAgent: true, MockClaudeCommand: "/tmp/mock-claude", MockCodexCommand: "/tmp/mock-codex"})
	if config.MockClaudeCommand != "/tmp/mock-claude" || config.MockCodexCommand != "/tmp/mock-codex" {
		t.Fatalf("mock 命令应支持 E2E 显式覆盖: claude=%q codex=%q", config.MockClaudeCommand, config.MockCodexCommand)
	}
	if !serverTestAgentProvidersContain(config.AgentProviders, wsapp.AgentProviderMockClaudeCode) || !serverTestAgentProvidersContain(config.AgentProviders, wsapp.AgentProviderMockCodex) {
		t.Fatalf("启用 MOCK_AGENT 后应返回 mock provider: %#v", config.AgentProviders)
	}
}

// serverTestAgentProvidersContain 使用 providers 和 provider 参数判断测试配置中是否包含指定 provider。
func serverTestAgentProvidersContain(providers []wsapp.AgentProviderOption, provider string) bool {
	for _, item := range providers {
		if item.ID == provider {
			return true
		}
	}
	return false
}

// TestAuthStatusSupportsSubpaths 验证鉴权状态接口支持根路径和子路径部署。
func TestAuthStatusSupportsSubpaths(t *testing.T) {
	server := httptest.NewServer(newHTTPHandler(context.Background(), webConfig{Port: "0", Token: "secret"}))
	defer server.Close()

	for _, requestPath := range []string{"/auth/status", "/console/auth/status"} {
		response, err := http.Get(server.URL + requestPath)
		if err != nil {
			t.Fatalf("请求鉴权状态失败: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("鉴权状态接口状态码应为 200，当前值: %d", response.StatusCode)
		}
		var body authStatusResponse
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatalf("解析鉴权状态失败: %v", err)
		}
		if !body.TokenRequired {
			t.Fatalf("配置 token 后应要求前端输入 token")
		}
	}
}

// TestTokenProtectedWebSocket 验证 WebSocket 需要正确 token 才能连接。
func TestTokenProtectedWebSocket(t *testing.T) {
	server := httptest.NewServer(newHTTPHandler(context.Background(), webConfig{Port: "0", Token: "secret"}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, response, err := websocket.Dial(ctx, testWebSocketURL(server.URL, "/ws"), nil)
	if err == nil {
		conn.CloseNow()
		t.Fatalf("缺少 token 时 WebSocket 不应连接成功")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("缺少 token 时应返回 401，当前响应: %+v，错误: %v", response, err)
	}

	conn, _, err = websocket.Dial(ctx, testWebSocketURL(server.URL, "/console/ws?token=secret"), nil)
	if err != nil {
		t.Fatalf("携带正确 token 时 WebSocket 应连接成功: %v", err)
	}
	defer conn.CloseNow()
	var snapshot wsapp.ServerMessage
	if err := wsjson.Read(ctx, conn, &snapshot); err != nil {
		t.Fatalf("读取状态快照失败: %v", err)
	}
	if snapshot.Type != "state.snapshot" {
		t.Fatalf("首条消息应为状态快照，当前值: %s", snapshot.Type)
	}
}

// TestFilesystemContentEndpoint 验证文件系统内容接口支持子路径、token 和绝对路径校验。
func TestFilesystemContentEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "demo.md")
	if err := os.WriteFile(filePath, []byte("# Demo\n\n文件内容"), 0600); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	server := httptest.NewServer(newHTTPHandler(context.Background(), webConfig{Port: "0", Token: "secret"}))
	defer server.Close()

	response, err := http.Get(server.URL + filesystemContentRoute + "?path=" + url.QueryEscape(filePath))
	if err != nil {
		t.Fatalf("请求缺少 token 的文件接口失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("缺少 token 时文件接口应返回 401，当前值: %d", response.StatusCode)
	}

	requestURL := server.URL + "/console" + filesystemContentRoute + "?token=secret&path=" + url.QueryEscape(filePath)
	response, err = http.Get(requestURL)
	if err != nil {
		t.Fatalf("请求文件接口失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("文件接口状态码应为 200，当前值: %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("读取文件接口响应失败: %v", err)
	}
	if !strings.Contains(string(body), "文件内容") {
		t.Fatalf("文件接口响应内容不正确: %s", string(body))
	}

	lineRequestURL := server.URL + "/console" + filesystemContentRoute + "?token=secret&path=" + url.QueryEscape(filePath+":1:2")
	response, err = http.Get(lineRequestURL)
	if err != nil {
		t.Fatalf("请求带行号文件接口失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("带行号文件接口状态码应为 200，当前值: %d", response.StatusCode)
	}
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("读取带行号文件接口响应失败: %v", err)
	}
	if !strings.Contains(string(body), "文件内容") {
		t.Fatalf("带行号文件接口响应内容不正确: %s", string(body))
	}

	response, err = http.Get(server.URL + filesystemContentRoute + "?token=secret&path=README.md")
	if err != nil {
		t.Fatalf("请求相对路径文件接口失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("文件接口应拒绝相对路径，当前状态码: %d", response.StatusCode)
	}
}

// testWebSocketURL 使用 baseURL 和 requestPath 参数生成测试 WebSocket 地址。
func testWebSocketURL(baseURL string, requestPath string) string {
	return "ws" + strings.TrimPrefix(baseURL, "http") + requestPath
}
