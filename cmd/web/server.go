package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/117503445/agenthub/internal/buildinfo"
	"github.com/117503445/agenthub/internal/wsapp"
)

// newHTTPHandler 使用 ctx 和 config 参数创建 WebSocket 服务的 HTTP 处理器。
func newHTTPHandler(ctx context.Context, config webConfig) http.Handler {
	handler, err := newHTTPHandlerWithError(ctx, config)
	if err != nil {
		panic(err)
	}
	return handler
}

// newHTTPHandlerWithError 使用 ctx 和 config 参数创建可返回错误的 HTTP 处理器。
func newHTTPHandlerWithError(ctx context.Context, config webConfig) (http.Handler, error) {
	mux := http.NewServeMux()
	var wsServer *wsapp.Server
	var err error
	if strings.TrimSpace(config.DataDir) == "" {
		wsServer = wsapp.NewServer(ctx, buildinfo.Version(), resolveAgentConfig(config.Port))
	} else {
		wsServer, err = wsapp.NewPersistentServer(ctx, buildinfo.Version(), resolveAgentConfig(config.Port), config.DataDir)
		if err != nil {
			return nil, err
		}
	}
	auth := newTokenAuth(config.Token)

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
	mux.HandleFunc("/mock/openai/v1/responses", wsapp.ServeMockOpenAIResponses)
	mux.HandleFunc("/mock/openai/responses", wsapp.ServeMockOpenAIResponses)
	mux.HandleFunc("/auth/status", auth.ServeStatus)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		auth.ServeWS(w, r, wsServer.ServeWS)
	})
	mux.Handle("/", subpathHandler(wsServer, auth, staticHandler()))

	return mux, nil
}

// subpathHandler 使用 wsServer、auth 和 static 参数处理子路径下的前端、鉴权状态与 WebSocket 请求。
func subpathHandler(wsServer *wsapp.Server, auth tokenAuth, static http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := path.Clean(r.URL.Path)
		if path.Base(requestPath) == "ws" {
			auth.ServeWS(w, r, wsServer.ServeWS)
			return
		}
		if isAuthStatusPath(requestPath) {
			auth.ServeStatus(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
}

// resolveAgentConfig 使用 port 参数生成 agent runtime 配置。
func resolveAgentConfig(port string) wsapp.AgentConfig {
	model := "sonnet"
	agentProviders := wsapp.AgentProviderOptions(wsapp.AgentOptionsConfig{
		ClaudeDefaultModel: model,
		CodexDefaultEffort: "xhigh",
	})
	mockBaseURL := backendMockBaseURL(port)
	return wsapp.AgentConfig{
		Command:              "claude",
		CodexCommand:         "codex",
		MockClaudeCommand:    "claude",
		MockCodexCommand:     "codex",
		AnthropicModel:       model,
		MockAnthropicBaseURL: mockBaseURL + "/mock/anthropic",
		MockAnthropicAPIKey:  "mock-key",
		MockOpenAIBaseURL:    mockBaseURL + "/mock/openai/v1",
		MockOpenAIAPIKey:     "mock-key",
		AgentProviders:       agentProviders,
	}
}

// backendMockBaseURL 使用 port 参数返回后端 mock 服务根地址。
func backendMockBaseURL(port string) string {
	return defaultAgentHubBaseURL(port)
}

// ListenAndServe 使用 ctx 和 config 参数记录日志并启动 HTTP 服务。
func ListenAndServe(ctx context.Context, config webConfig) error {
	if strings.TrimSpace(config.DataDir) == "" {
		dataDir, err := resolveWebDataDir("")
		if err != nil {
			return err
		}
		config.DataDir = dataDir
	}
	dataLock, err := acquireDataDirLock(ctx, config.DataDir)
	if err != nil {
		return err
	}
	defer dataLock.Release()

	handler, err := newHTTPHandlerWithError(ctx, config)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", ":"+config.Port)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("监听端口失败")
		return err
	}
	defer func() {
		if err := listener.Close(); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("关闭监听器失败")
		}
	}()

	log.Ctx(ctx).Info().
		Str("addr", listener.Addr().String()).
		Str("dataDir", config.DataDir).
		Msg("Web 服务已启动")
	return http.Serve(listener, handler)
}
