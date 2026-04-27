// Package buildweb 实现 Web 前端嵌入资源和后端二进制构建命令。
package buildweb

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/117503445/coding/scripts/go-scripts/common"
)

// Run 运行 build-web 命令。
func Run() error {
	return BuildBinary(filepath.Join("data", "web", "web"), os.Stdout, os.Stderr)
}

// BuildBinary 构建包含前端资源的 WebSocket 后端二进制，outputPath 参数指定二进制输出路径，stdout 和 stderr 参数接收构建日志。
func BuildBinary(outputPath string, stdout io.Writer, stderr io.Writer) error {
	distDir := filepath.Join("cmd", "web", "dist")
	backupDir, err := os.MkdirTemp("", "coding-web-dist-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(backupDir)

	if err := common.CopyDir(distDir, filepath.Join(backupDir, "dist")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("备份静态资源失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(distDir)
		_ = os.MkdirAll(distDir, 0755)
		_ = common.CopyDir(filepath.Join(backupDir, "dist"), distDir)
	}()

	if err := os.RemoveAll(distDir); err != nil {
		return fmt.Errorf("清理静态资源目录失败: %w", err)
	}
	if err := os.MkdirAll(distDir, 0755); err != nil {
		return fmt.Errorf("创建静态资源目录失败: %w", err)
	}
	if err := common.CopyDir(filepath.Join("fe", "dist"), distDir); err != nil {
		return fmt.Errorf("复制前端产物失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	args := []string{"build", "-o", outputPath}
	if ldflags := buildLDFlags(); ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "./cmd/web")
	return common.RunWithWriters(stdout, stderr, "go", args...)
}

// buildLDFlags 生成构建信息注入参数。
func buildLDFlags() string {
	pairs := map[string]string{
		"BuildTime":  time.Now().Format(time.RFC3339),
		"GitBranch":  gitOutput("rev-parse", "--abbrev-ref", "HEAD"),
		"GitCommit":  gitOutput("rev-parse", "HEAD"),
		"GitTag":     gitOutput("describe", "--tags", "--abbrev=0"),
		"GitDirty":   gitDirty(),
		"GitVersion": gitOutput("describe", "--tags", "--always", "--dirty"),
		"BuildDir":   mustGetwd(),
	}

	parts := make([]string, 0, len(pairs))
	for key, value := range pairs {
		parts = append(parts, fmt.Sprintf("-X github.com/117503445/coding/internal/buildinfo.%s=%s", key, value))
	}
	return strings.Join(parts, " ")
}

// gitOutput 执行 args 参数指定的 git 命令并返回裁剪后的输出。
func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// gitDirty 返回当前工作区是否存在未提交内容。
func gitDirty() string {
	cmd := exec.Command("git", "status", "--short")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%t", strings.TrimSpace(string(output)) != "")
}

// mustGetwd 返回当前工作目录，失败时返回空字符串。
func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}
