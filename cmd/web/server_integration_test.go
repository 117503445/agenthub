//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/117503445/coding/internal/wsapp"
)

// TestIntegrationHealthz 验证 t 参数提供的集成测试上下文中健康检查接口可用。
func TestIntegrationHealthz(t *testing.T) {
	server := httptest.NewServer(newHTTPHandler(context.Background()))
	defer server.Close()

	resp, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatalf("请求健康检查失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("健康检查状态码异常: %d", resp.StatusCode)
	}

	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		Time    string `json:"time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("解析健康检查响应失败: %v", err)
	}
	if !body.OK || body.Version == "" || body.Time == "" {
		t.Fatalf("健康检查响应内容异常: %+v", body)
	}
}

// TestIntegrationWebSocketEcho 验证 t 参数提供的集成测试上下文中 WebSocket 回环接口可用。
func TestIntegrationWebSocketEcho(t *testing.T) {
	server := httptest.NewServer(newHTTPHandler(context.Background()))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer conn.CloseNow()

	var hello wsapp.ServerMessage
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		t.Fatalf("读取欢迎消息失败: %v", err)
	}
	if hello.Type != "hello" {
		t.Fatalf("欢迎消息类型异常: %s", hello.Type)
	}

	message := "集成测试消息"
	if err := wsjson.Write(ctx, conn, wsapp.ClientMessage{
		Type:    "echo",
		Payload: message,
	}); err != nil {
		t.Fatalf("发送回环消息失败: %v", err)
	}

	var echo wsapp.ServerMessage
	if err := wsjson.Read(ctx, conn, &echo); err != nil {
		t.Fatalf("读取回环消息失败: %v", err)
	}
	if echo.Type != "echo" || echo.Payload["echo"] != message {
		t.Fatalf("回环消息异常: %+v", echo)
	}
}
