package main

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

	"github.com/117503445/coding/internal/wsapp"
)

// runUnitTests 运行单元测试，并把日志写入 data/ut。
func runUnitTests() error {
	logPath := filepath.Join("data", "ut", "test.log")
	logFile, err := recreateLogFile(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	output := io.MultiWriter(os.Stdout, logFile)
	if _, err := fmt.Fprintln(output, "运行单元测试: go test ./..."); err != nil {
		return err
	}
	return runWithWriters(output, output, "go", "test", "./...")
}

// runIntegrationTests 编译并启动 Go 服务，再用 Go client 验证服务行为。
func runIntegrationTests() error {
	dir := filepath.Join("data", "it")
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("清理集成测试目录失败: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建集成测试目录失败: %w", err)
	}

	buildLog, err := recreateLogFile(filepath.Join(dir, "build.log"))
	if err != nil {
		return err
	}
	defer buildLog.Close()
	buildOutput := io.MultiWriter(os.Stdout, buildLog)
	binaryPath := filepath.Join(dir, "web")
	if _, err := fmt.Fprintln(buildOutput, "编译集成测试服务"); err != nil {
		return err
	}
	if err := buildWebBinary(binaryPath, buildOutput, buildOutput); err != nil {
		return err
	}

	serverLog, err := recreateLogFile(filepath.Join(dir, "server.log"))
	if err != nil {
		return err
	}
	defer serverLog.Close()

	clientLog, err := recreateLogFile(filepath.Join(dir, "client.log"))
	if err != nil {
		return err
	}
	defer clientLog.Close()
	clientOutput := io.MultiWriter(os.Stdout, clientLog)

	port, err := findFreeTCPPort()
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	serverCmd := exec.Command(binaryPath)
	serverCmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		"CODING_LOG_NO_COLOR=true",
	)
	serverCmd.Stdout = serverLog
	serverCmd.Stderr = serverLog
	if err := serverCmd.Start(); err != nil {
		return fmt.Errorf("启动集成测试服务失败: %w", err)
	}
	defer stopProcess(serverCmd)

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
	if err := verifyWebSocketEcho(ctx, baseURL, clientOutput); err != nil {
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

// verifyWebSocketEcho 使用 ctx 参数控制 Go WebSocket client 调用，验证 baseURL 参数对应的消息回环，并把结果写入 logWriter 参数。
func verifyWebSocketEcho(ctx context.Context, baseURL string, logWriter io.Writer) error {
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("连接 WebSocket 失败: %w", err)
	}
	defer conn.CloseNow()

	var hello wsapp.ServerMessage
	if err := wsjson.Read(ctx, conn, &hello); err != nil {
		return fmt.Errorf("读取欢迎消息失败: %w", err)
	}
	if hello.Type != "hello" {
		return fmt.Errorf("欢迎消息类型异常: %s", hello.Type)
	}

	message := "集成测试消息"
	if err := wsjson.Write(ctx, conn, wsapp.ClientMessage{
		Type:    "echo",
		Payload: message,
	}); err != nil {
		return fmt.Errorf("发送回环消息失败: %w", err)
	}

	var echo wsapp.ServerMessage
	if err := wsjson.Read(ctx, conn, &echo); err != nil {
		return fmt.Errorf("读取回环消息失败: %w", err)
	}
	if echo.Type != "echo" || echo.Payload["echo"] != message {
		return fmt.Errorf("回环消息异常: %+v", echo)
	}
	_, err = fmt.Fprintf(logWriter, "WebSocket 回环通过: %s\n", message)
	return err
}
