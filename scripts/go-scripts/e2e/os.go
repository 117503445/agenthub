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
	return waitForCondition(fmt.Sprintf("等待文件 %s 包含 %q", path, text), timeout, func() (bool, string, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return false, "", err
		}
		content := string(data)
		return strings.Contains(content, text), "当前内容: " + content, nil
	})
}
