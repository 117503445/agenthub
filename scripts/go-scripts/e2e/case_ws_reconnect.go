package e2e

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/117503445/agenthub/scripts/go-scripts/common"
)

// runWSReconnectCase 使用 ctx 参数运行 WebSocket 自动重连用例。
func runWSReconnectCase(ctx E2EContext) (success bool) {
	ctx.Logger.Infof("打开页面: %s", ctx.BaseURL)
	events := make([]reportEvent, 0)
	defer func() {
		writeWSReconnectReport(ctx.OutputDir, success, events)
	}()

	session, err := newBrowserSession(1366, 900)
	if err != nil {
		ctx.Logger.Errorf("创建浏览器失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("WebSocket 自动重连 E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		events = append(events, reportImage("失败现场", "screenshots/failed.png"))
		return false
	}

	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	projectName := filepath.Base(ctx.RootDir)
	if err := expectTestIDText(page, "project-list", projectName, 10*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("页面首次连接后收到后端状态快照。"))

	ctx.StopServer()
	if err := expectTestIDNotText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("后端停止后，前端连接状态离开已连接。"))

	restartStop, err := restartServerAtSameBaseURL(ctx, "reconnect-logs")
	if err != nil {
		return fail(err)
	}
	defer restartStop()
	if err := waitUntilReady(ctx.BaseURL); err != nil {
		return fail(fmt.Errorf("重启服务未就绪: %w", err))
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 20*time.Second); err != nil {
		return fail(err)
	}
	if err := expectTestIDText(page, "project-list", projectName, 10*time.Second); err != nil {
		return fail(err)
	}

	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-reconnected.png"), true)
	events = append(events, reportStep("后端在同一地址重启后，前端自动重连并重新渲染状态快照。"))
	events = append(events, reportImage("WebSocket 自动重连", "screenshots/01-reconnected.png"))
	return true
}

// restartServerAtSameBaseURL 使用 ctx 和 logDirName 参数在原端口重启服务。
func restartServerAtSameBaseURL(ctx E2EContext, logDirName string) (func(), error) {
	port, err := portFromBaseURL(ctx.BaseURL)
	if err != nil {
		return nil, err
	}
	legacyPort, err := common.FindFreeTCPPort()
	if err != nil {
		return nil, err
	}
	logsDir := filepath.Join(ctx.OutputDir, logDirName)
	_, stop, err := startServer(ctx.RootDir, ctx.ServerCmd, port, legacyPort, logsDir, ctx.DataDir, nil, nil)
	if err != nil {
		return nil, err
	}
	return stop, nil
}

// portFromBaseURL 使用 baseURL 参数解析 HTTP 服务端口。
func portFromBaseURL(baseURL string) (int, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return 0, err
	}
	portText := parsed.Port()
	if portText == "" {
		return 0, fmt.Errorf("baseURL 缺少端口: %s", baseURL)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return 0, fmt.Errorf("baseURL 端口不合法 %q: %w", portText, err)
	}
	return port, nil
}

// writeWSReconnectReport 使用 outputDir、success 和 events 参数写入 WebSocket 重连测试报告。
func writeWSReconnectReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "WebSocket 自动重连 E2E 测试报告", success, events)
}
