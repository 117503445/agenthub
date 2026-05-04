package runweb

import (
	"os"
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

// TestRunPortUsesDefaultPort 验证运行脚本缺省使用默认端口。
func TestRunPortUsesDefaultPort(t *testing.T) {
	unsetEnv(t, "AGENTHUB_PORT")

	port, err := runPort()
	if err != nil {
		t.Fatalf("读取运行端口失败: %v", err)
	}
	if port != defaultPort {
		t.Fatalf("默认端口应为 %s，当前值: %q", defaultPort, port)
	}
}
