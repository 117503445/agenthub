// Package ut 实现单元测试命令。
package ut

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/117503445/coding/scripts/go-scripts/common"
)

// Run 运行单元测试，并把日志写入 data/ut。
func Run() error {
	logPath := filepath.Join("data", "ut", "test.log")
	logFile, err := common.RecreateLogFile(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	output := io.MultiWriter(os.Stdout, logFile)
	if _, err := fmt.Fprintln(output, "运行单元测试: go test ./..."); err != nil {
		return err
	}
	return common.RunWithWriters(output, output, "go", "test", "./...")
}
