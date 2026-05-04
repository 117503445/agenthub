package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unsetEnv 使用 t 和 key 参数临时移除环境变量。
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("移除环境变量失败: %v", err)
	}
	t.Cleanup(func() {
		if !ok {
			_ = os.Unsetenv(key)
			return
		}
		_ = os.Setenv(key, value)
	})
}

// TestParseWebConfigUsesOnlyAgentHubEnv 验证 Web 配置只读取 AGENTHUB 前缀环境变量。
func TestParseWebConfigUsesOnlyAgentHubEnv(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("AGENTHUB_PORT", "6060")
	t.Setenv("AGENTHUB_LOG_NO_COLOR", "true")
	t.Setenv("AGENTHUB_TOKEN", "secret-token")
	t.Setenv("AGENTHUB_DATA", "~/custom-data")
	t.Setenv("AGENTHUB_MOCK_CLAUDE_COMMAND", "/tmp/mock-claude")
	t.Setenv("AGENTHUB_MOCK_CODEX_COMMAND", "/tmp/mock-codex")
	unsetEnv(t, "MOCK_AGENT")
	t.Setenv("PORT", "7070")
	t.Setenv("LEGACY_AGENTHUB_LOG_NO_COLOR", "false")
	t.Setenv("LEGACY_AGENTHUB_TOKEN", "legacy-token")

	config, err := parseWebConfig(nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if config.Port != "6060" {
		t.Fatalf("端口应来自 AGENTHUB_PORT，当前值: %q", config.Port)
	}
	if !config.LogNoColor {
		t.Fatalf("AGENTHUB_LOG_NO_COLOR=true 应禁用日志颜色")
	}
	if config.Token != "secret-token" {
		t.Fatalf("token 应来自 AGENTHUB_TOKEN，当前值: %q", config.Token)
	}
	expectedDataDir := filepath.Join(homeDir, "custom-data")
	if config.DataDir != expectedDataDir {
		t.Fatalf("数据目录应来自 AGENTHUB_DATA 并展开 ~，当前值: %q", config.DataDir)
	}
	if config.MockClaudeCommand != "/tmp/mock-claude" || config.MockCodexCommand != "/tmp/mock-codex" {
		t.Fatalf("mock 命令应来自 AGENTHUB 环境变量: claude=%q codex=%q", config.MockClaudeCommand, config.MockCodexCommand)
	}
	if config.MockAgent {
		t.Fatalf("未设置 MOCK_AGENT 时不应启用 Mock Agent")
	}
}

// TestParseWebConfigEnablesMockAgent 验证 MOCK_AGENT 环境变量可以启用 Mock Agent。
func TestParseWebConfigEnablesMockAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MOCK_AGENT", "1")

	config, err := parseWebConfig(nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if !config.MockAgent {
		t.Fatalf("MOCK_AGENT=1 应启用 Mock Agent")
	}
}

// TestParseWebConfigFlagOverridesEnv 验证 CLI 参数优先级高于环境变量。
func TestParseWebConfigFlagOverridesEnv(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTHUB_PORT", "6060")
	t.Setenv("AGENTHUB_TOKEN", "env-token")

	config, err := parseWebConfig([]string{"--port", "7070", "--token", "cli-token"}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if config.Port != "7070" {
		t.Fatalf("CLI 端口应覆盖环境变量，当前值: %q", config.Port)
	}
	if config.Token != "cli-token" {
		t.Fatalf("CLI token 应覆盖环境变量，当前值: %q", config.Token)
	}
}

// TestParseWebConfigUsesDefaultPort 验证未配置端口时使用默认端口。
func TestParseWebConfigUsesDefaultPort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	unsetEnv(t, "AGENTHUB_PORT")

	config, err := parseWebConfig(nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if config.Port != defaultWebPort {
		t.Fatalf("默认端口应为 %s，当前值: %q", defaultWebPort, config.Port)
	}
}

// TestParseWebConfigDefaultDataDir 验证默认数据目录位于用户目录下。
func TestParseWebConfigDefaultDataDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	config, err := parseWebConfig(nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	expectedDataDir := filepath.Join(homeDir, ".agenthub", "data")
	if config.DataDir != expectedDataDir {
		t.Fatalf("默认数据目录不正确: %q", config.DataDir)
	}
}

// TestWebCLIHelpHidesLogNoColor 验证 Kong 帮助信息隐藏日志颜色环境变量。
func TestWebCLIHelpHidesLogNoColor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := printWebHelp(&stdout, &stderr); err != nil {
		t.Fatalf("输出帮助失败: %v", err)
	}
	help := stdout.String() + stderr.String()
	if !strings.Contains(help, "AGENTHUB_PORT") {
		t.Fatalf("帮助信息应展示 AGENTHUB_PORT，当前帮助: %s", help)
	}
	if !strings.Contains(help, "--token") {
		t.Fatalf("帮助信息应展示 token 参数，当前帮助: %s", help)
	}
	if strings.Contains(help, "AGENTHUB_LOG_NO_COLOR") || strings.Contains(help, "log-no-color") {
		t.Fatalf("帮助信息不应展示日志颜色配置，当前帮助: %s", help)
	}
	if strings.Contains(help, "AGENTHUB_TOKEN") {
		t.Fatalf("帮助信息不应展示 token 环境变量，当前帮助: %s", help)
	}
}
