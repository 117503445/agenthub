package e2e

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/117503445/agenthub/scripts/go-scripts/common"
	"github.com/playwright-community/playwright-go"
)

// E2ECase 表示一个可运行的 E2E 用例。
type E2ECase struct {
	Name string                // Name 表示用例名称。
	Run  func(E2EContext) bool // Run 使用 E2EContext 参数运行用例。
	Env  map[string]string     // Env 表示当前用例额外覆盖的环境变量。
	Args []string              // Args 表示当前用例额外传递给服务的 CLI 参数。
}

// E2EContext 表示 E2E 用例运行所需上下文。
type E2EContext struct {
	BaseURL        string      // BaseURL 表示被测服务根地址。
	OutputDir      string      // OutputDir 表示当前用例输出目录。
	DataDir        string      // DataDir 表示当前用例使用的后端数据目录。
	ScreenshotsDir string      // ScreenshotsDir 表示当前用例截图目录。
	LogsDir        string      // LogsDir 表示当前用例日志目录。
	Logger         *caseLogger // Logger 表示当前用例日志器。
	RootDir        string      // RootDir 表示项目根目录。
	ServerCmd      string      // ServerCmd 表示当前用例使用的服务启动命令。
	StopServer     func()      // StopServer 停止当前用例自动启动的服务。
}

// caseLogger 包装标准日志器，统一输出到终端和用例日志文件。
type caseLogger struct {
	logger *log.Logger
}

// Infof 使用 format 和 args 参数写入普通日志。
func (l *caseLogger) Infof(format string, args ...any) {
	l.logger.Printf("INFO - "+format, args...)
}

// Errorf 使用 format 和 args 参数写入错误日志。
func (l *caseLogger) Errorf(format string, args ...any) {
	l.logger.Printf("ERROR - "+format, args...)
}

// Run 使用 args 参数解析命令行并运行 E2E。
func Run(args []string) int {
	rootDir, err := common.RootDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "定位项目根目录失败: %v\n", err)
		return 1
	}

	flags := flag.NewFlagSet("e2e", flag.ContinueOnError)
	caseName := flags.String("case", "", "指定要运行的用例")
	serverCmd := flags.String("server-cmd", filepath.Join(rootDir, "data", "web", "web"), "服务启动命令")
	if err := flags.Parse(args); err != nil {
		return 1
	}

	if err := ensurePlaywright(); err != nil {
		fmt.Fprintf(os.Stderr, "初始化 Playwright 失败: %v\n", err)
		return 1
	}

	cases := registeredCases()
	caseNames := make([]string, 0, len(cases))
	for name := range cases {
		caseNames = append(caseNames, name)
	}
	sort.Strings(caseNames)
	if *caseName != "" {
		if _, ok := cases[*caseName]; !ok {
			fmt.Fprintf(os.Stderr, "用例不存在: %s\n", *caseName)
			return 1
		}
		caseNames = []string{*caseName}
	}

	results := make(map[string]bool, len(caseNames))
	for _, name := range caseNames {
		results[name] = runCase(rootDir, *serverCmd, cases[name])
	}
	allPassed := true
	for _, name := range caseNames {
		if results[name] {
			log.Printf("%s: PASSED", name)
			continue
		}
		log.Printf("%s: FAILED", name)
		allPassed = false
	}
	if !allPassed {
		return 1
	}
	return 0
}

// registeredCases 返回当前仓库内置 E2E 用例。
func registeredCases() map[string]E2ECase {
	return map[string]E2ECase{
		"case_agent_chat": {Name: "case_agent_chat", Run: runAgentChatCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_agent_provider_empty_profile": {Name: "case_agent_provider_empty_profile", Run: runAgentProviderEmptyProfileCase, Env: map[string]string{
			"MOCK_AGENT": "",
		}},
		"case_chat_send_feedback": {Name: "case_chat_send_feedback", Run: runChatSendFeedbackCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_chat_detail_lazy_load": {Name: "case_chat_detail_lazy_load", Run: runChatDetailLazyLoadCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_codex_resume_context": {Name: "case_codex_resume_context", Run: runCodexResumeContextCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_codex_timeline_events": {Name: "case_codex_timeline_events", Run: runCodexTimelineEventsCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_codex_dash_prompt": {Name: "case_codex_dash_prompt", Run: runCodexDashPromptCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_manual_stop_notification": {Name: "case_manual_stop_notification", Run: runManualStopNotificationCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_mock_agent_gate": {Name: "case_mock_agent_gate", Run: runMockAgentGateCase},
		"case_persistence":     {Name: "case_persistence", Run: runPersistenceCase},
		"case_markdown_filesystem_links": {Name: "case_markdown_filesystem_links", Run: runMarkdownFilesystemLinksCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_markdown_ordered_list": {Name: "case_markdown_ordered_list", Run: runMarkdownOrderedListCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_agent_profile": {Name: "case_agent_profile", Run: runAgentProfileCase, Env: map[string]string{
			"AGENTHUB_PROFILE_E2E_SECRET": "backend-secret-value",
			"AGENTHUB_PROFILE_E2E_UNSET":  "backend-unset-value",
			"MOCK_AGENT":                  "1",
		}},
		"case_material_theme":          {Name: "case_material_theme", Run: runMaterialThemeCase},
		"case_project_chat_tab_memory": {Name: "case_project_chat_tab_memory", Run: runProjectChatTabMemoryCase},
		"case_project_icon":            {Name: "case_project_icon", Run: runProjectIconCase},
		"case_project_reorder":         {Name: "case_project_reorder", Run: runProjectReorderCase},
		"case_plan_request_user_input": {Name: "case_plan_request_user_input", Run: runPlanRequestUserInputCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_settings_backend_version": {Name: "case_settings_backend_version", Run: runSettingsBackendVersionCase},
		"case_skill_command":            {Name: "case_skill_command", Run: runSkillCommandCase},
		"case_stream_coalescing": {Name: "case_stream_coalescing", Run: runStreamCoalescingCase, Env: map[string]string{
			"MOCK_AGENT": "1",
		}},
		"case_token_auth": {Name: "case_token_auth", Run: runTokenAuthCase, Env: map[string]string{
			"AGENTHUB_TOKEN": e2eAgentHubToken,
		}},
		"case_token_cli_auth": {Name: "case_token_cli_auth", Run: runTokenAuthCase, Args: []string{
			"--token", e2eAgentHubToken,
		}},
		"case_ws":           {Name: "case_ws", Run: runWSCase},
		"case_ws_reconnect": {Name: "case_ws_reconnect", Run: runWSReconnectCase},
	}
}

// ensurePlaywright 确保 Playwright driver 和 Chromium 已安装。
func ensurePlaywright() error {
	return Install(io.Discard, io.Discard)
}

// Install 使用 stdout 和 stderr 参数输出安装日志，并安装 Playwright Chromium 运行环境。
func Install(stdout io.Writer, stderr io.Writer) error {
	return playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
		Stdout:   stdout,
		Stderr:   stderr,
	})
}

// runCase 使用 rootDir、serverCmd 和 item 参数运行单个 E2E 用例。
func runCase(rootDir string, serverCmd string, item E2ECase) bool {
	outputDir := filepath.Join(rootDir, "data", "e2e", item.Name)
	dataDir := filepath.Join(outputDir, "agenthub-data")
	screenshotsDir := filepath.Join(outputDir, "screenshots")
	logsDir := filepath.Join(outputDir, "logs")
	if err := os.RemoveAll(outputDir); err != nil {
		log.Printf("清理用例输出目录失败: %v", err)
		return false
	}
	for _, dir := range []string{screenshotsDir, logsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("创建用例输出目录失败: %v", err)
			return false
		}
	}

	testLog, err := os.Create(filepath.Join(logsDir, "test.log"))
	if err != nil {
		log.Printf("创建测试日志失败: %v", err)
		return false
	}
	defer testLog.Close()

	logger := &caseLogger{logger: log.New(io.MultiWriter(os.Stdout, testLog), "", log.LstdFlags)}
	port, err := common.FindFreeTCPPort()
	if err != nil {
		logger.Errorf("查找可用端口失败: %v", err)
		return false
	}
	legacyPort, err := common.FindFreeTCPPort()
	if err != nil {
		logger.Errorf("查找旧环境变量冲突端口失败: %v", err)
		return false
	}
	for legacyPort == port {
		legacyPort, err = common.FindFreeTCPPort()
		if err != nil {
			logger.Errorf("重新查找旧环境变量冲突端口失败: %v", err)
			return false
		}
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	process, stopServer, err := startServer(rootDir, serverCmd, port, legacyPort, logsDir, dataDir, item.Env, item.Args)
	if err != nil {
		logger.Errorf("启动服务失败: %v", err)
		return false
	}
	stopOnce := sync.OnceFunc(stopServer)
	defer stopOnce()
	if err := waitUntilReady(baseURL); err != nil {
		logger.Errorf("服务未就绪: %v", err)
		if process.ProcessState != nil {
			logger.Errorf("服务退出状态: %v", process.ProcessState)
		}
		return false
	}

	logger.Infof("E2E 目标地址: %s", baseURL)
	logger.Infof("开始运行用例: %s", item.Name)
	success := item.Run(E2EContext{
		BaseURL:        baseURL,
		OutputDir:      outputDir,
		DataDir:        dataDir,
		ScreenshotsDir: screenshotsDir,
		LogsDir:        logsDir,
		Logger:         logger,
		RootDir:        rootDir,
		ServerCmd:      serverCmd,
		StopServer:     stopOnce,
	})
	logger.Infof("用例 %s 结果: %s", item.Name, passText(success))
	return success
}

// startServer 使用 rootDir、serverCmd、port、legacyPort、logsDir、dataDir、extraEnv 和 extraArgs 参数启动被测服务。
func startServer(
	rootDir string,
	serverCmd string,
	port int,
	legacyPort int,
	logsDir string,
	dataDir string,
	extraEnv map[string]string,
	extraArgs []string,
) (*exec.Cmd, func(), error) {
	parts := strings.Fields(serverCmd)
	if len(parts) == 0 {
		return nil, nil, fmt.Errorf("server-cmd 不能为空")
	}
	mockCodexCommand, mockClaudeCommand, err := prepareMockAgentCommands(rootDir, parts[0])
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, nil, err
	}
	serverLog, err := os.Create(filepath.Join(logsDir, "server.log"))
	if err != nil {
		return nil, nil, err
	}
	homeDir, err := prepareE2EHome(rootDir)
	if err != nil {
		_ = serverLog.Close()
		return nil, nil, err
	}

	args := append([]string{}, parts[1:]...)
	args = append(args, extraArgs...)
	cmd := exec.Command(parts[0], args...)
	cmd.Dir = rootDir
	envOverride := map[string]string{
		"AGENTHUB_PORT":                fmt.Sprintf("%d", port),
		"AGENTHUB_LOG_NO_COLOR":        "true",
		"AGENTHUB_TOKEN":               "",
		"AGENTHUB_DATA":                dataDir,
		"AGENTHUB_MOCK_CODEX_COMMAND":  mockCodexCommand,
		"AGENTHUB_MOCK_CLAUDE_COMMAND": mockClaudeCommand,
		"PORT":                         fmt.Sprintf("%d", legacyPort),
		"HOME":                         homeDir,
	}
	for key, value := range extraEnv {
		envOverride[key] = value
	}
	cmd.Env = withEnvOverride(os.Environ(), envOverride)
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

// prepareMockAgentCommands 使用 rootDir 和 serverPath 参数准备 E2E mock agent 命令。
func prepareMockAgentCommands(rootDir string, serverPath string) (string, string, error) {
	absServerPath, err := filepath.Abs(serverPath)
	if err != nil {
		return "", "", err
	}
	mockDir := filepath.Join(rootDir, "data", "e2e", "mock-agents")
	if err := os.MkdirAll(mockDir, 0755); err != nil {
		return "", "", err
	}
	mockCodexPath := filepath.Join(mockDir, "mock-codex")
	mockClaudePath := filepath.Join(mockDir, "mock-claude")
	if err := recreateMockAgentLink(absServerPath, mockCodexPath); err != nil {
		return "", "", err
	}
	if err := recreateMockAgentLink(absServerPath, mockClaudePath); err != nil {
		return "", "", err
	}
	return mockCodexPath, mockClaudePath, nil
}

// prepareE2EHome 使用 rootDir 参数准备 E2E 专用用户目录和测试 skills。
func prepareE2EHome(rootDir string) (string, error) {
	homeDir := filepath.Join(rootDir, "data", "e2e", "home")
	skillsDir := filepath.Join(homeDir, ".codex", "skills")
	for _, dir := range []string{
		skillsDir,
		filepath.Join(homeDir, ".claude", "skills"),
		filepath.Join(homeDir, ".agents", "skills"),
	} {
		if err := os.RemoveAll(dir); err != nil {
			return "", err
		}
	}
	if err := writeE2ESkill(skillsDir, "e2e-alpha", "E2E 键盘选择 Alpha。"); err != nil {
		return "", err
	}
	if err := writeE2ESkill(skillsDir, "e2e-beta", "E2E 键盘选择 Beta。"); err != nil {
		return "", err
	}
	if err := writeE2ESkill(skillsDir, "e2e-skill", "E2E 技能选择测试。"); err != nil {
		return "", err
	}
	return homeDir, nil
}

// writeE2ESkill 使用 skillsDir、name 和 description 参数写入测试 skill。
func writeE2ESkill(skillsDir string, name string, description string) error {
	skillDir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return err
	}
	content := strings.Join([]string{
		"---",
		"name: " + name,
		"description: " + description,
		"---",
		"",
		"# " + name,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		return err
	}
	return nil
}

// recreateMockAgentLink 使用 target 和 linkPath 参数重建 mock agent 链接。
func recreateMockAgentLink(target string, linkPath string) error {
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(target, linkPath); err == nil {
		return nil
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if err := os.WriteFile(linkPath, data, 0755); err != nil {
		return err
	}
	return nil
}

// withEnvOverride 使用 env 和 override 参数生成覆盖后的环境变量列表。
func withEnvOverride(env []string, override map[string]string) []string {
	result := make([]string, 0, len(env)+len(override))
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replaced := override[key]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for key, value := range override {
		result = append(result, key+"="+value)
	}
	return result
}

// waitUntilReady 使用 baseURL 参数等待健康检查通过。
func waitUntilReady(baseURL string) error {
	return waitUntilReadyWithin(baseURL, 20*time.Second)
}

// waitUntilReadyWithin 使用 baseURL 和 timeout 参数在限定时间内等待健康检查通过。
func waitUntilReadyWithin(baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: time.Second}
	return waitForCondition("等待健康检查通过", timeout, func() (bool, string, error) {
		response, err := client.Get(baseURL + "/healthz")
		if err != nil {
			return false, "", err
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusOK {
			return true, "", nil
		}
		return false, fmt.Sprintf("状态码 %d", response.StatusCode), nil
	})
}

// passText 使用 success 参数返回中文结果文本。
func passText(success bool) string {
	if success {
		return "通过"
	}
	return "失败"
}
