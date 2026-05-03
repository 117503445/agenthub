package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
)

const defaultWebPort = "8080"

// webConfig 表示 Web 服务启动配置。
type webConfig struct {
	Port       string // Port 表示 Web 服务监听端口。
	LogNoColor bool   // LogNoColor 表示是否禁用日志颜色。
	Token      string // Token 表示前端访问 WebSocket 时需要提供的 token。
}

// webCLI 表示 Kong 解析的命令行参数。
type webCLI struct {
	Port       string `name:"port" env:"AGENTHUB_PORT" default:"8080" help:"Web 服务监听端口。"`
	LogNoColor bool   `name:"log-no-color" env:"AGENTHUB_LOG_NO_COLOR" hidden:"" help:"禁用日志颜色。"`
}

// parseWebConfig 使用 args 参数解析 Web 服务启动配置。
func parseWebConfig(args []string, stdout io.Writer, stderr io.Writer) (webConfig, error) {
	var cli webCLI
	parser, err := newWebParser(&cli, stdout, stderr)
	if err != nil {
		return webConfig{}, err
	}
	if _, err := parser.Parse(args); err != nil {
		return webConfig{}, err
	}
	if err := validateWebPort(cli.Port); err != nil {
		return webConfig{}, err
	}
	return webConfig{
		Port:       strings.TrimSpace(cli.Port),
		LogNoColor: cli.LogNoColor,
		Token:      strings.TrimSpace(os.Getenv("AGENTHUB_TOKEN")),
	}, nil
}

// printWebHelp 使用 stdout 和 stderr 参数输出 Kong 帮助信息。
func printWebHelp(stdout io.Writer, stderr io.Writer) error {
	var cli webCLI
	parser, err := newWebParser(&cli, stdout, stderr)
	if err != nil {
		return err
	}
	parser.Exit = func(int) {}
	_, err = parser.Parse([]string{"--help"})
	return err
}

// newWebParser 使用 cli、stdout 和 stderr 参数创建 Kong 解析器。
func newWebParser(cli *webCLI, stdout io.Writer, stderr io.Writer) (*kong.Kong, error) {
	return kong.New(cli,
		kong.Name("agenthub"),
		kong.Description("AgentHub Web 服务。"),
		kong.Writers(stdout, stderr),
	)
}

// validateWebPort 验证 port 参数是否为合法 TCP 端口。
func validateWebPort(port string) error {
	trimmed := strings.TrimSpace(port)
	value, err := strconv.Atoi(trimmed)
	if err != nil || value <= 0 || value > 65535 {
		return fmt.Errorf("AGENTHUB_PORT 必须是 1-65535 的端口号，当前值: %q", port)
	}
	return nil
}

// defaultAgentHubBaseURL 使用 port 参数返回默认本机服务地址。
func defaultAgentHubBaseURL(port string) string {
	trimmedPort := strings.TrimSpace(port)
	if trimmedPort == "" {
		trimmedPort = defaultWebPort
	}
	return "http://127.0.0.1:" + trimmedPort
}
