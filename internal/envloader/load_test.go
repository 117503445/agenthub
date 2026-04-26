package envloader

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad 验证 t 参数提供的测试上下文中 .env 文件加载行为。
func TestLoad(t *testing.T) {
	existingKey := "CODING_TEST_EXISTING"
	newKey := "CODING_TEST_NEW"
	t.Setenv(existingKey, "keep")
	t.Cleanup(func() {
		_ = os.Unsetenv(newKey)
	})

	envPath := filepath.Join(t.TempDir(), ".env")
	content := "# 注释行\n" +
		existingKey + "=override\n" +
		newKey + "=\"loaded value\"\n" +
		"INVALID_LINE\n"
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("写入 .env 失败: %v", err)
	}

	if err := Load(envPath); err != nil {
		t.Fatalf("加载 .env 失败: %v", err)
	}
	if got := os.Getenv(existingKey); got != "keep" {
		t.Fatalf("已有环境变量被覆盖: %s", got)
	}
	if got := os.Getenv(newKey); got != "loaded value" {
		t.Fatalf("新环境变量未正确加载: %s", got)
	}
}

// TestLoadMissing 验证 t 参数提供的测试上下文中缺失文件不会报错。
func TestLoadMissing(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), ".env")
	if err := Load(missingPath); err != nil {
		t.Fatalf("缺失 .env 不应报错: %v", err)
	}
}
