package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/117503445/agenthub/internal/wsapp"
	"github.com/117503445/agenthub/scripts/go-scripts/common"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// persistenceClientMessage 表示持久化 E2E 发送到 WebSocket 的消息。
type persistenceClientMessage struct {
	Type    string          `json:"type"`              // Type 表示消息类型。
	Payload json.RawMessage `json:"payload,omitempty"` // Payload 表示消息正文。
}

// persistenceServerMessage 表示持久化 E2E 从 WebSocket 收到的消息。
type persistenceServerMessage struct {
	Type    string          `json:"type"`              // Type 表示消息类型。
	Payload json.RawMessage `json:"payload,omitempty"` // Payload 表示消息正文。
}

// persistenceProjectChangedPayload 表示 project.changed 消息正文。
type persistenceProjectChangedPayload struct {
	Project wsapp.Project `json:"project"` // Project 表示变更后的 project。
}

// runPersistenceCase 使用 ctx 参数运行后端 JSON 持久化用例。
func runPersistenceCase(ctx E2EContext) (success bool) {
	steps := make([]string, 0)
	defer func() {
		writePersistenceReport(ctx.OutputDir, success, steps)
	}()

	fail := func(err error) bool {
		ctx.Logger.Errorf("后端持久化 E2E 失败: %v", err)
		steps = append(steps, fmt.Sprintf("用例失败: %v", err))
		return false
	}

	projectDir := filepath.Join(ctx.OutputDir, "persisted-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fail(err)
	}

	conn, err := dialPersistenceWS(ctx.BaseURL)
	if err != nil {
		return fail(err)
	}
	defer conn.CloseNow()
	if _, err := readPersistenceMessage(conn, "state.snapshot", 5*time.Second); err != nil {
		return fail(err)
	}
	project, err := createPersistenceProject(conn, projectDir)
	if err != nil {
		return fail(err)
	}
	steps = append(steps, "通过 WebSocket 创建 project，服务端返回 project.changed。")

	statePath := filepath.Join(ctx.DataDir, "state.json")
	if err := waitPersistenceState(statePath, project.Path, 5*time.Second); err != nil {
		return fail(err)
	}
	steps = append(steps, "写操作完成后，AGENTHUB_DATA 下立即出现 state.json，并包含新增 project。")

	if err := assertSameDataDirSecondProcessExits(ctx); err != nil {
		return fail(err)
	}
	steps = append(steps, "原服务存活时，同一个 AGENTHUB_DATA 的第二个进程会自行退出。")

	conn.CloseNow()
	ctx.StopServer()
	restartBaseURL, restartStop, err := startPersistenceServer(ctx, "restart-logs")
	if err != nil {
		return fail(err)
	}
	defer restartStop()
	if err := waitUntilReady(restartBaseURL); err != nil {
		return fail(fmt.Errorf("重启服务未就绪: %w", err))
	}
	restartConn, err := dialPersistenceWS(restartBaseURL)
	if err != nil {
		return fail(err)
	}
	defer restartConn.CloseNow()
	snapshotMessage, err := readPersistenceMessage(restartConn, "state.snapshot", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	if err := assertPersistenceSnapshotHasProject(snapshotMessage.Payload, project.Path); err != nil {
		return fail(err)
	}
	steps = append(steps, "服务重启后从 state.json 恢复 project，并在首个状态快照中返回。")
	return true
}

// dialPersistenceWS 使用 baseURL 参数连接后端 WebSocket。
func dialPersistenceWS(baseURL string) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(baseURL, "http")+"/ws", nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// createPersistenceProject 使用 conn 和 projectPath 参数创建 project。
func createPersistenceProject(conn *websocket.Conn, projectPath string) (wsapp.Project, error) {
	payload, err := json.Marshal(wsapp.ProjectMutationPayload{Path: projectPath})
	if err != nil {
		return wsapp.Project{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, conn, persistenceClientMessage{
		Type:    "project.create",
		Payload: payload,
	}); err != nil {
		return wsapp.Project{}, err
	}
	message, err := readPersistenceMessage(conn, "project.changed", 5*time.Second)
	if err != nil {
		return wsapp.Project{}, err
	}
	var changed persistenceProjectChangedPayload
	if err := json.Unmarshal(message.Payload, &changed); err != nil {
		return wsapp.Project{}, err
	}
	return changed.Project, nil
}

// readPersistenceMessage 使用 conn、messageType 和 timeout 参数等待指定消息。
func readPersistenceMessage(conn *websocket.Conn, messageType string, timeout time.Duration) (persistenceServerMessage, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(context.Background(), time.Until(deadline))
		var message persistenceServerMessage
		err := wsjson.Read(readCtx, conn, &message)
		cancel()
		if err != nil {
			return persistenceServerMessage{}, err
		}
		if message.Type == messageType {
			return message, nil
		}
	}
	return persistenceServerMessage{}, fmt.Errorf("等待 WebSocket 消息 %s 超时", messageType)
}

// waitPersistenceState 使用 statePath、projectPath 和 timeout 参数等待 state.json 包含 project。
func waitPersistenceState(statePath string, projectPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := assertPersistenceStateHasProject(statePath, projectPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	return lastErr
}

// assertPersistenceStateHasProject 使用 statePath 和 projectPath 参数断言 state.json 包含 project 和聊天页。
func assertPersistenceStateHasProject(statePath string, projectPath string) error {
	data, err := os.ReadFile(statePath)
	if err != nil {
		return err
	}
	var state struct {
		Projects []wsapp.Project `json:"projects"` // Projects 表示持久化的 project 列表。
		Chats    []wsapp.Chat    `json:"chats"`    // Chats 表示持久化的聊天页列表。
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	for _, project := range state.Projects {
		if project.Path == projectPath {
			for _, chat := range state.Chats {
				if chat.ProjectID == project.ID {
					return nil
				}
			}
			return fmt.Errorf("state.json 中 project %s 缺少聊天页", projectPath)
		}
	}
	return fmt.Errorf("state.json 未包含 project: %s", projectPath)
}

// assertSameDataDirSecondProcessExits 使用 ctx 参数验证同一数据目录第二进程自行退出。
func assertSameDataDirSecondProcessExits(ctx E2EContext) error {
	port, err := common.FindFreeTCPPort()
	if err != nil {
		return err
	}
	legacyPort, err := common.FindFreeTCPPort()
	if err != nil {
		return err
	}
	lockLogsDir := filepath.Join(ctx.OutputDir, "lock-logs")
	process, stop, err := startServer(ctx.RootDir, ctx.ServerCmd, port, legacyPort, lockLogsDir, ctx.DataDir, nil)
	if err != nil {
		return err
	}
	defer stop()
	if err := waitProcessExit(process, 5*time.Second); err != nil {
		return err
	}
	if isHealthReady(fmt.Sprintf("http://127.0.0.1:%d", port)) {
		return fmt.Errorf("同数据目录第二进程不应提供健康检查")
	}
	return nil
}

// startPersistenceServer 使用 ctx 和 logDirName 参数启动同一数据目录的新服务。
func startPersistenceServer(ctx E2EContext, logDirName string) (string, func(), error) {
	port, err := common.FindFreeTCPPort()
	if err != nil {
		return "", nil, err
	}
	legacyPort, err := common.FindFreeTCPPort()
	if err != nil {
		return "", nil, err
	}
	logsDir := filepath.Join(ctx.OutputDir, logDirName)
	_, stop, err := startServer(ctx.RootDir, ctx.ServerCmd, port, legacyPort, logsDir, ctx.DataDir, nil)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), stop, nil
}

// waitProcessExit 使用 process 和 timeout 参数等待进程退出。
func waitProcessExit(process *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- process.Wait()
	}()
	select {
	case <-time.After(timeout):
		return fmt.Errorf("进程未在 %s 内退出", timeout)
	case err := <-done:
		if err != nil {
			return nil
		}
		return nil
	}
}

// isHealthReady 使用 baseURL 参数判断健康检查是否可用。
func isHealthReady(baseURL string) bool {
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get(baseURL + "/healthz")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

// assertPersistenceSnapshotHasProject 使用 payload 和 projectPath 参数断言快照包含 project。
func assertPersistenceSnapshotHasProject(payload json.RawMessage, projectPath string) error {
	var snapshot wsapp.Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return err
	}
	for _, project := range snapshot.Projects {
		if project.Path == projectPath {
			return nil
		}
	}
	return fmt.Errorf("重启快照未包含 project: %s", projectPath)
}

// writePersistenceReport 使用 outputDir、success 和 steps 参数写入持久化测试报告。
func writePersistenceReport(outputDir string, success bool, steps []string) {
	report := []string{
		"# 后端 JSON 持久化 E2E 测试报告",
		"",
		fmt.Sprintf("- 结果: %s", passText(success)),
		"- 日志: [test.log](logs/test.log)",
		"- 服务日志: [server.log](logs/server.log)",
		"",
		"## 步骤",
		"",
	}
	for _, step := range steps {
		report = append(report, "- "+step)
	}
	writeFile(filepath.Join(outputDir, "report.md"), strings.Join(report, "\n")+"\n")
}
