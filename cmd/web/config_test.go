package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestParseWebConfigUsesOnlyAgentHubEnv 验证 Web 配置只读取 AGENTHUB 前缀环境变量。
func TestParseWebConfigUsesOnlyAgentHubEnv(t *testing.T) {
	t.Setenv("AGENTHUB_PORT", "6060")
	t.Setenv("AGENTHUB_LOG_NO_COLOR", "true")
	t.Setenv("AGENTHUB_TOKEN", "secret-token")
	t.Setenv("PORT", "7070")
	t.Setenv("CODING_LOG_NO_COLOR", "false")
	t.Setenv("CODING_TOKEN", "legacy-token")

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
}

// TestParseWebConfigFlagOverridesEnv 验证 CLI 参数优先级高于环境变量。
func TestParseWebConfigFlagOverridesEnv(t *testing.T) {
	t.Setenv("AGENTHUB_PORT", "6060")

	config, err := parseWebConfig([]string{"--port", "7070"}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if config.Port != "7070" {
		t.Fatalf("CLI 端口应覆盖环境变量，当前值: %q", config.Port)
	}
}

// TestWebCLIHelpHidesLogNoColor 验证 Kong 帮助信息隐藏日志颜色环境变量。
func TestWebCLIHelpHidesLogNoColor(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := printWebHelp(&stdout, &stderr); err != nil {
		t.Fatalf("输出帮助失败: %v", err)
	}
	help := stdout.String() + stderr.String()
	if !strings.Contains(help, "AGENTHUB_PORT") {
		t.Fatalf("帮助信息应展示 AGENTHUB_PORT，当前帮助: %s", help)
	}
	if strings.Contains(help, "AGENTHUB_LOG_NO_COLOR") || strings.Contains(help, "log-no-color") {
		t.Fatalf("帮助信息不应展示日志颜色配置，当前帮助: %s", help)
	}
	if strings.Contains(help, "AGENTHUB_TOKEN") || strings.Contains(help, "token") {
		t.Fatalf("帮助信息不应展示 token 配置，当前帮助: %s", help)
	}
}
