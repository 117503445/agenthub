// Package runweb 提供本地 Web 服务运行前的准备逻辑。
package runweb

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultPort     = "8080"
	stopWaitTimeout = 3 * time.Second
	killWaitTimeout = 2 * time.Second
)

var ssPIDPattern = regexp.MustCompile(`pid=([0-9]+)`)

// Prepare 使用 stdout 和 stderr 参数输出日志，清理 PORT 端口上已有的监听进程。
func Prepare(stdout io.Writer, stderr io.Writer) error {
	port, err := runPort()
	if err != nil {
		return err
	}
	if isPortFree(port) {
		return nil
	}

	pids, err := listeningPIDs(port)
	if err != nil {
		return err
	}
	if len(pids) == 0 {
		return fmt.Errorf("端口 %s 已被占用，但未找到监听进程", port)
	}

	fmt.Fprintf(stdout, "端口 %s 已被占用，准备结束旧进程: %s\n", port, joinPIDs(pids))
	if err := signalPIDs(pids, syscall.SIGTERM); err != nil {
		return err
	}
	if waitPortFree(port, stopWaitTimeout) {
		return nil
	}

	fmt.Fprintf(stderr, "旧进程未及时退出，准备强制结束: %s\n", joinPIDs(pids))
	if err := signalPIDs(pids, syscall.SIGKILL); err != nil {
		return err
	}
	if !waitPortFree(port, killWaitTimeout) {
		return fmt.Errorf("端口 %s 仍被占用", port)
	}
	return nil
}

// runPort 从环境变量读取运行端口，缺省时返回默认端口。
func runPort() (string, error) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	value, err := strconv.Atoi(port)
	if err != nil || value <= 0 || value > 65535 {
		return "", fmt.Errorf("PORT 必须是 1-65535 的端口号，当前值: %q", port)
	}
	return port, nil
}

// isPortFree 判断 port 参数对应的 TCP 端口是否可监听。
func isPortFree(port string) bool {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

// waitPortFree 等待 port 参数对应的 TCP 端口在 timeout 参数内释放。
func waitPortFree(port string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isPortFree(port) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return isPortFree(port)
}

// listeningPIDs 返回 port 参数对应 TCP 监听端口上的进程 PID 列表。
func listeningPIDs(port string) ([]int, error) {
	pids, err := pidsByLsof(port)
	if err == nil {
		return pids, nil
	}
	lsofErr := err

	pids, err = pidsByFuser(port)
	if err == nil {
		return pids, nil
	}
	fuserErr := err

	pids, err = pidsBySS(port)
	if err == nil {
		return pids, nil
	}
	return nil, fmt.Errorf("查询端口 %s 监听进程失败: lsof: %v; fuser: %v; ss: %v", port, lsofErr, fuserErr, err)
}

// pidsByLsof 使用 lsof 查询 port 参数对应 TCP 监听端口的进程 PID。
func pidsByLsof(port string) ([]int, error) {
	cmd := exec.Command("lsof", "-nP", "-tiTCP:"+port, "-sTCP:LISTEN")
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	return parsePIDs(string(output)), nil
}

// pidsByFuser 使用 fuser 查询 port 参数对应 TCP 监听端口的进程 PID。
func pidsByFuser(port string) ([]int, error) {
	cmd := exec.Command("fuser", "-n", "tcp", port)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return parsePIDs(string(output)), nil
}

// pidsBySS 使用 ss 查询 port 参数对应 TCP 监听端口的进程 PID。
func pidsBySS(port string) ([]int, error) {
	cmd := exec.Command("ss", "-H", "-ltnp", "sport", "=", ":"+port)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseSSPIDs(string(output)), nil
}

// parsePIDs 从 output 参数中解析 PID 列表。
func parsePIDs(output string) []int {
	seen := make(map[int]bool)
	pids := make([]int, 0)
	for _, field := range strings.Fields(output) {
		pid, ok := parsePIDField(field)
		if !ok || seen[pid] || pid == os.Getpid() {
			continue
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	return pids
}

// parseSSPIDs 从 output 参数中解析 ss 输出里的 PID 列表。
func parseSSPIDs(output string) []int {
	seen := make(map[int]bool)
	pids := make([]int, 0)
	for _, match := range ssPIDPattern.FindAllStringSubmatch(output, -1) {
		if len(match) != 2 {
			continue
		}
		pid, err := strconv.Atoi(match[1])
		if err != nil || pid <= 0 || seen[pid] || pid == os.Getpid() {
			continue
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	return pids
}

// parsePIDField 从 field 参数中解析一个 PID。
func parsePIDField(field string) (int, bool) {
	end := 0
	for end < len(field) && field[end] >= '0' && field[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	pid, err := strconv.Atoi(field[:end])
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// signalPIDs 向 pids 参数中的进程发送 signal 参数指定的信号。
func signalPIDs(pids []int, signal syscall.Signal) error {
	for _, pid := range pids {
		process, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("查找进程 %d 失败: %w", pid, err)
		}
		if err := process.Signal(signal); err != nil && !isProcessGone(err) {
			return fmt.Errorf("结束进程 %d 失败: %w", pid, err)
		}
	}
	return nil
}

// isProcessGone 判断 err 参数是否表示进程已经退出。
func isProcessGone(err error) bool {
	return errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH)
}

// joinPIDs 把 pids 参数格式化为逗号分隔的字符串。
func joinPIDs(pids []int) string {
	parts := make([]string, 0, len(pids))
	for _, pid := range pids {
		parts = append(parts, strconv.Itoa(pid))
	}
	return strings.Join(parts, ", ")
}
