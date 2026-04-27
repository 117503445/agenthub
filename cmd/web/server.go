package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/117503445/coding/internal/buildinfo"
	"github.com/117503445/coding/internal/wsapp"
)

// newHTTPHandler 使用 ctx 和 port 参数创建 WebSocket 服务的 HTTP 处理器。
func newHTTPHandler(ctx context.Context, port string) http.Handler {
	mux := http.NewServeMux()
	wsServer := wsapp.NewServer(ctx, buildinfo.Version(), resolveAgentConfig())

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"version": buildinfo.Version(),
			"time":    time.Now().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/mock/anthropic/v1/messages/count_tokens", wsapp.ServeMockAnthropicCountTokens)
	mux.HandleFunc("/mock/anthropic/v1/messages", wsapp.ServeMockAnthropicMessages)
	mux.HandleFunc("/ws", wsServer.ServeWS)
	mux.Handle("/", subpathHandler(wsServer, staticHandler()))

	return mux
}

// subpathHandler 使用 wsServer 和 static 参数处理子路径下的前端与 WebSocket 请求。
func subpathHandler(wsServer *wsapp.Server, static http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path.Base(path.Clean(r.URL.Path)) == "ws" {
			wsServer.ServeWS(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
}

// resolveAgentConfig 生成 agent runtime 配置。
func resolveAgentConfig() wsapp.AgentConfig {
	baseURL := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL"))
	model := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
	if model == "" {
		model = "sonnet"
	}
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	command := strings.TrimSpace(os.Getenv("CODING_CLAUDE_COMMAND"))
	if command == "" {
		command = "claude"
	}
	codexCommand := strings.TrimSpace(os.Getenv("CODING_CODEX_COMMAND"))
	if codexCommand == "" {
		codexCommand = "codex"
	}
	mockClaudeCommand := strings.TrimSpace(os.Getenv("CODING_MOCK_CLAUDE_COMMAND"))
	if mockClaudeCommand == "" {
		mockClaudeCommand = command
	}
	mockCodexCommand := strings.TrimSpace(os.Getenv("CODING_MOCK_CODEX_COMMAND"))
	if mockCodexCommand == "" {
		mockCodexCommand = codexCommand
	}
	agentProviders := wsapp.AgentProviderOptions(wsapp.AgentOptionsConfig{
		ClaudeDefaultModel: model,
		CodexDefaultEffort: "xhigh",
	})
	return wsapp.AgentConfig{
		Command:           command,
		CodexCommand:      codexCommand,
		MockClaudeCommand: mockClaudeCommand,
		MockCodexCommand:  mockCodexCommand,
		AnthropicBaseURL:  baseURL,
		AnthropicModel:    model,
		AnthropicAPIKey:   apiKey,
		AgentProviders:    agentProviders,
	}
}

// ListenAndServe 使用 ctx 参数记录日志，并在 port 参数指定的端口启动 HTTP 服务。
func ListenAndServe(ctx context.Context, port string) error {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("监听端口失败")
		return err
	}
	defer func() {
		if err := listener.Close(); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("关闭监听器失败")
		}
	}()

	log.Ctx(ctx).Info().Str("addr", listener.Addr().String()).Msg("Web 服务已启动")
	return http.Serve(listener, newHTTPHandler(ctx, port))
}
