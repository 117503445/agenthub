package e2e

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// osStat 使用 path 参数读取文件状态，集中封装便于用例表达。
func osStat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// expectFileText 使用 path、text 和 timeout 参数等待文件内容包含指定文本。
func expectFileText(path string, text string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastContent string
	var lastErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			lastContent = string(data)
			if strings.Contains(lastContent, text) {
				return nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("等待文件 %s 包含 %q 超时，最后错误: %w", path, text, lastErr)
	}
	return fmt.Errorf("等待文件 %s 包含 %q 超时，当前内容: %s", path, text, lastContent)
}
