package main

import (
	"fmt"
	"os"
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
	case "ut":
		if err := runUnitTests(); err != nil {
			exitWithError(err)
		}
	case "it":
		if err := runIntegrationTests(); err != nil {
			exitWithError(err)
		}
	default:
		exitWithError(fmt.Errorf("未知脚本命令: %s", os.Args[1]))
	}
}
