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

// Server 提供基于 coder/websocket 的消息处理服务。
type Server struct {
	version string
}

// NewServer 使用 version 参数创建 WebSocket 服务。
func NewServer(version string) *Server {
	return &Server{version: version}
}

// ServeWS 使用 w 和 r 参数升级 HTTP 请求并处理 WebSocket 消息。
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

// handle 根据 msg 参数中的类型处理连接 conn 上的业务消息。
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

// write 把 messageType 和 payload 参数封装为服务端消息并写入连接 conn。
func (s *Server) write(ctx context.Context, conn *websocket.Conn, messageType string, payload map[string]any) error {
	return wsjson.Write(ctx, conn, ServerMessage{
		Type:       messageType,
		Payload:    payload,
		ServerTime: time.Now().Format(time.RFC3339),
		Version:    s.version,
	})
}

// isExpectedClose 判断 err 参数是否属于正常关闭。
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
