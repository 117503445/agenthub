package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// main 解析脚本命令并执行对应任务。
func main() {
	if len(os.Args) < 2 {
		exitWithError(fmt.Errorf("缺少脚本命令"))
	}

	switch os.Args[1] {
	case "build-web":
		if err := buildWeb(); err != nil {
			exitWithError(err)
		}
	default:
		exitWithError(fmt.Errorf("未知脚本命令: %s", os.Args[1]))
	}
}

// buildWeb 构建前端嵌入资源和 WebSocket 后端二进制。
func buildWeb() error {
	distDir := filepath.Join("cmd", "web", "dist")
	backupDir, err := os.MkdirTemp("", "coding-web-dist-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(backupDir)

	if err := copyDir(distDir, filepath.Join(backupDir, "dist")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("备份静态资源失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(distDir)
		_ = os.MkdirAll(distDir, 0755)
		_ = copyDir(filepath.Join(backupDir, "dist"), distDir)
	}()

	if err := os.RemoveAll(distDir); err != nil {
		return fmt.Errorf("清理静态资源目录失败: %w", err)
	}
	if err := os.MkdirAll(distDir, 0755); err != nil {
		return fmt.Errorf("创建静态资源目录失败: %w", err)
	}
	if err := copyDir(filepath.Join("fe", "dist"), distDir); err != nil {
		return fmt.Errorf("复制前端产物失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join("data", "web"), 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	args := []string{"build", "-o", filepath.Join("data", "web", "web")}
	if ldflags := buildLDFlags(); ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "./cmd/web")
	return run("go", args...)
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

// gitOutput 执行 git 参数并返回裁剪后的输出。
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

// run 执行 name 和 args 参数组成的命令。
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行命令失败 %s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// copyDir 把 src 参数目录递归复制到 dst 参数目录。
func copyDir(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("源路径不是目录: %s", src)
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

// copyFile 把 src 参数文件复制到 dst 参数路径，并使用 mode 参数设置权限。
func copyFile(src string, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer output.Close()

	_, err = io.Copy(output, input)
	return err
}

// exitWithError 输出 err 参数并以失败状态退出。
func exitWithError(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
