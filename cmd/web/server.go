package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/117503445/coding/internal/buildinfo"
	"github.com/117503445/coding/internal/wsapp"
)

// newHTTPHandler 使用 ctx 参数创建 WebSocket 服务的 HTTP 处理器。
func newHTTPHandler(ctx context.Context) http.Handler {
	mux := http.NewServeMux()
	wsServer := wsapp.NewServer(buildinfo.Version())

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"version": buildinfo.Version(),
			"time":    time.Now().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/ws", wsServer.ServeWS)
	mux.Handle("/", staticHandler())

	_ = ctx
	return mux
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
	return http.Serve(listener, newHTTPHandler(ctx))
}
