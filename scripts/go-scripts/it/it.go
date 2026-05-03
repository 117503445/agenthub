// Package it 实现 Web 后端集成测试命令。
package it

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/117503445/agenthub/internal/wsapp"
	"github.com/117503445/agenthub/scripts/go-scripts/buildweb"
	"github.com/117503445/agenthub/scripts/go-scripts/common"
)

// Run 编译并启动 Go 服务，再用 Go client 验证服务行为。
func Run() error {
	dir := filepath.Join("data", "it")
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("清理集成测试目录失败: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建集成测试目录失败: %w", err)
	}

	buildLog, err := common.RecreateLogFile(filepath.Join(dir, "build.log"))
	if err != nil {
		return err
	}
	defer buildLog.Close()
	buildOutput := io.MultiWriter(os.Stdout, buildLog)
	binaryPath := filepath.Join(dir, "web")
	if _, err := fmt.Fprintln(buildOutput, "编译集成测试服务"); err != nil {
		return err
	}
	if err := buildweb.BuildBinary(binaryPath, buildOutput, buildOutput); err != nil {
		return err
	}

	serverLog, err := common.RecreateLogFile(filepath.Join(dir, "server.log"))
	if err != nil {
		return err
	}
	defer serverLog.Close()

	clientLog, err := common.RecreateLogFile(filepath.Join(dir, "client.log"))
	if err != nil {
		return err
	}
	defer clientLog.Close()
	clientOutput := io.MultiWriter(os.Stdout, clientLog)

	port, err := common.FindFreeTCPPort()
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	serverCmd := exec.Command(binaryPath)
	serverCmd.Env = append(os.Environ(),
		fmt.Sprintf("AGENTHUB_PORT=%d", port),
		"AGENTHUB_LOG_NO_COLOR=true",
	)
	serverCmd.Stdout = serverLog
	serverCmd.Stderr = serverLog
	if err := serverCmd.Start(); err != nil {
		return fmt.Errorf("启动集成测试服务失败: %w", err)
	}
	defer common.StopProcess(serverCmd)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := fmt.Fprintf(clientOutput, "等待服务就绪: %s\n", baseURL); err != nil {
		return err
	}
	if err := waitServiceReady(ctx, baseURL, serverCmd); err != nil {
		return err
	}
	if err := verifyHealthz(ctx, baseURL, clientOutput); err != nil {
		return err
	}
	if err := verifyWebSocketState(ctx, baseURL, clientOutput); err != nil {
		return err
	}
	_, err = fmt.Fprintln(clientOutput, "集成测试通过")
	return err
}

// waitServiceReady 在 ctx 参数限定时间内等待 baseURL 参数对应的服务健康检查通过，并用 serverCmd 参数判断服务是否提前退出。
func waitServiceReady(ctx context.Context, baseURL string, serverCmd *exec.Cmd) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		if serverCmd.ProcessState != nil && serverCmd.ProcessState.Exited() {
			return fmt.Errorf("服务提前退出: %s", serverCmd.ProcessState.String())
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("等待服务就绪超时: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// verifyHealthz 使用 ctx 参数控制 Go HTTP client 调用，验证 baseURL 参数对应的健康检查接口，并把结果写入 logWriter 参数。
func verifyHealthz(ctx context.Context, baseURL string, logWriter io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求健康检查失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("健康检查状态码异常: %d", resp.StatusCode)
	}
	var body struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		Time    string `json:"time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("解析健康检查响应失败: %w", err)
	}
	if !body.OK || body.Version == "" || body.Time == "" {
		return fmt.Errorf("健康检查响应内容异常: %+v", body)
	}
	_, err = fmt.Fprintf(logWriter, "健康检查通过: version=%s time=%s\n", body.Version, body.Time)
	return err
}

// verifyWebSocketState 使用 ctx 参数控制 Go WebSocket client 调用，验证 baseURL 参数对应的状态快照和 ping 响应，并把结果写入 logWriter 参数。
func verifyWebSocketState(ctx context.Context, baseURL string, logWriter io.Writer) error {
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("连接 WebSocket 失败: %w", err)
	}
	defer conn.CloseNow()

	var snapshot wsapp.ServerMessage
	if err := wsjson.Read(ctx, conn, &snapshot); err != nil {
		return fmt.Errorf("读取状态快照失败: %w", err)
	}
	if snapshot.Type != "state.snapshot" {
		return fmt.Errorf("状态快照消息类型异常: %s", snapshot.Type)
	}

	if err := wsjson.Write(ctx, conn, wsapp.ClientMessage{
		Type: "ping",
	}); err != nil {
		return fmt.Errorf("发送 ping 消息失败: %w", err)
	}

	var pong wsapp.ServerMessage
	if err := wsjson.Read(ctx, conn, &pong); err != nil {
		return fmt.Errorf("读取 pong 消息失败: %w", err)
	}
	if pong.Type != "pong" {
		return fmt.Errorf("pong 消息异常: %+v", pong)
	}
	_, err = fmt.Fprintln(logWriter, "WebSocket 状态快照和 ping 通过")
	return err
}
