package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runWithWriters 执行 name 和 args 参数组成的命令，并写入 stdout 和 stderr 参数。
func runWithWriters(stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("执行命令失败 %s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// findFreeTCPPort 查找本机可用 TCP 端口。
func findFreeTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("查找可用端口失败: %w", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("端口地址类型异常: %T", listener.Addr())
	}
	return addr.Port, nil
}

// stopProcess 停止 cmd 参数对应的进程，超时后强制结束。
func stopProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return
	}

	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	case err := <-done:
		if err != nil && !errors.Is(err, os.ErrProcessDone) {
			return
		}
	}
}

// recreateLogFile 重新创建 path 参数指定的日志文件。
func recreateLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("创建日志文件失败: %w", err)
	}
	return file, nil
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
