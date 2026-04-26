package wsapp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/rs/zerolog/log"
)

type Server struct {
	version string
}

func NewServer(version string) *Server {
	return &Server{version: version}
}

func (s *Server) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
	})
	if err != nil {
		log.Error().Err(err).Msg("接受 WebSocket 连接失败")
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4096)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.write(ctx, conn, "hello", map[string]any{
		"message": "WebSocket 已连接",
	}); err != nil {
		log.Error().Err(err).Msg("发送欢迎消息失败")
		return
	}

	for {
		var msg ClientMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			if isExpectedClose(err) {
				log.Info().Msg("WebSocket 连接已关闭")
				return
			}
			log.Error().Err(err).Msg("读取 WebSocket 消息失败")
			return
		}

		if err := s.handle(ctx, conn, msg); err != nil {
			log.Error().Err(err).Str("type", msg.Type).Msg("处理 WebSocket 消息失败")
			return
		}
	}
}

func (s *Server) handle(ctx context.Context, conn *websocket.Conn, msg ClientMessage) error {
	switch msg.Type {
	case "ping":
		return s.write(ctx, conn, "pong", map[string]any{
			"message": "pong",
			"echo":    msg.Payload,
		})
	case "echo":
		return s.write(ctx, conn, "echo", map[string]any{
			"message": "后端已收到",
			"echo":    msg.Payload,
		})
	case "time":
		return s.write(ctx, conn, "time", map[string]any{
			"message": "服务器时间已刷新",
		})
	default:
		return s.write(ctx, conn, "error", map[string]any{
			"message": "不支持的消息类型",
			"type":    msg.Type,
		})
	}
}

func (s *Server) write(ctx context.Context, conn *websocket.Conn, messageType string, payload map[string]any) error {
	return wsjson.Write(ctx, conn, ServerMessage{
		Type:       messageType,
		Payload:    payload,
		ServerTime: time.Now().Format(time.RFC3339),
		Version:    s.version,
	})
}

func isExpectedClose(err error) bool {
	if err == nil {
		return true
	}
	status := websocket.CloseStatus(err)
	return errors.Is(err, context.Canceled) ||
		status == websocket.StatusNormalClosure ||
		status == websocket.StatusGoingAway ||
		status == websocket.StatusNoStatusRcvd
}
