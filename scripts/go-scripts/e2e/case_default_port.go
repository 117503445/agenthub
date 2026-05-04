package e2e

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/117503445/agenthub/scripts/go-scripts/common"
)

const expectedDefaultWebPort = "17375"

// runDefaultPortCase 使用 ctx 参数验证服务缺省监听新默认端口。
func runDefaultPortCase(ctx E2EContext) bool {
	events := make([]reportEvent, 0)
	success := false
	defer func() {
		writeDefaultPortReport(ctx.OutputDir, success, events)
	}()

	ctx.StopServer()
	if !isLocalTCPPortFree(expectedDefaultWebPort) {
		events = append(events, reportStep("默认端口 "+expectedDefaultWebPort+" 已被占用，无法验证缺省监听行为。"))
		return false
	}
	legacyPort, err := findLegacyPort(expectedDefaultWebPort)
	if err != nil {
		events = append(events, reportStep(fmt.Sprintf("查找旧环境变量端口失败: %v", err)))
		return false
	}

	dataDir := filepath.Join(ctx.OutputDir, "default-port-data")
	process, stopServer, err := startDefaultPortServer(ctx, dataDir, legacyPort)
	if err != nil {
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}
	defer stopServer()

	defaultBaseURL := "http://127.0.0.1:" + expectedDefaultWebPort
	if err := waitUntilReadyWithin(defaultBaseURL, 5*time.Second); err != nil {
		if process.ProcessState != nil {
			events = append(events, reportStep(fmt.Sprintf("服务退出状态: %v", process.ProcessState)))
		}
		events = append(events, reportStep(fmt.Sprintf("默认端口 %s 健康检查失败: %v", expectedDefaultWebPort, err)))
		return false
	}

	events = append(events, reportStep("未设置 AGENTHUB_PORT 时，服务在默认端口 "+expectedDefaultWebPort+" 通过健康检查。"))
	success = true
	return true
}

// writeDefaultPortReport 使用 outputDir、success 和 events 参数写入默认端口测试报告。
func writeDefaultPortReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "默认端口 E2E 测试报告", success, events)
}

// startDefaultPortServer 使用 ctx、dataDir 和 legacyPort 参数启动未设置 AGENTHUB_PORT 的被测服务。
func startDefaultPortServer(ctx E2EContext, dataDir string, legacyPort string) (*exec.Cmd, func(), error) {
	parts := strings.Fields(ctx.ServerCmd)
	if len(parts) == 0 {
		return nil, nil, fmt.Errorf("server-cmd 不能为空")
	}
	serverPath, err := filepath.Abs(parts[0])
	if err != nil {
		return nil, nil, err
	}
	mockCodexCommand, mockClaudeCommand, err := prepareMockAgentCommands(ctx.RootDir, serverPath)
	if err != nil {
		return nil, nil, err
	}
	homeDir, err := prepareE2EHome(ctx.RootDir)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(ctx.LogsDir, 0755); err != nil {
		return nil, nil, err
	}
	workDir := filepath.Join(ctx.OutputDir, "default-port-workdir")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, nil, err
	}
	serverLog, err := os.Create(filepath.Join(ctx.LogsDir, "default-port-server.log"))
	if err != nil {
		return nil, nil, err
	}

	cmd := exec.Command(serverPath, parts[1:]...)
	cmd.Dir = workDir
	cmd.Env = withEnvOverride(withoutEnvKeys(os.Environ(), "AGENTHUB_PORT"), map[string]string{
		"AGENTHUB_LOG_NO_COLOR":        "true",
		"AGENTHUB_TOKEN":               "",
		"AGENTHUB_DATA":                dataDir,
		"AGENTHUB_MOCK_CODEX_COMMAND":  mockCodexCommand,
		"AGENTHUB_MOCK_CLAUDE_COMMAND": mockClaudeCommand,
		"PORT":                         legacyPort,
		"HOME":                         homeDir,
	})
	cmd.Stdout = serverLog
	cmd.Stderr = serverLog
	if err := cmd.Start(); err != nil {
		_ = serverLog.Close()
		return nil, nil, err
	}

	stop := func() {
		common.StopProcess(cmd)
		_ = serverLog.Close()
	}
	return cmd, stop, nil
}

// findLegacyPort 使用 expectedPort 参数查找与默认端口不同的旧环境变量端口。
func findLegacyPort(expectedPort string) (string, error) {
	for {
		port, err := common.FindFreeTCPPort()
		if err != nil {
			return "", err
		}
		legacyPort := fmt.Sprintf("%d", port)
		if legacyPort != expectedPort {
			return legacyPort, nil
		}
	}
}

// withoutEnvKeys 使用 env 和 keys 参数返回移除指定环境变量后的列表。
func withoutEnvKeys(env []string, keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	result := make([]string, 0, len(env))
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, skip := blocked[key]; skip {
				continue
			}
		}
		result = append(result, item)
	}
	return result
}

// isLocalTCPPortFree 使用 port 参数判断本机 TCP 端口是否可监听。
func isLocalTCPPortFree(port string) bool {
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}
