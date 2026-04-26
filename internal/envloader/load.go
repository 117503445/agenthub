package envloader

import (
	"bufio"
	"os"
	"strings"
)

// LoadDefault 加载项目根目录的 .env 文件。
func LoadDefault() error {
	return Load(".env")
}

// Load 加载 path 参数指定的 env 文件，已存在的环境变量不会被覆盖。
func Load(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, trimValue(value)); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// trimValue 清理 value 参数中的空白和可选引号。
func trimValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
