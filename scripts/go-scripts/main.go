package main

import (
	"fmt"
	"os"

	"github.com/117503445/coding/scripts/go-scripts/buildweb"
	"github.com/117503445/coding/scripts/go-scripts/e2e"
	"github.com/117503445/coding/scripts/go-scripts/it"
	"github.com/117503445/coding/scripts/go-scripts/ut"
)

// main 解析脚本命令并执行对应任务。
func main() {
	if len(os.Args) < 2 {
		exitWithError(fmt.Errorf("缺少脚本命令"))
	}

	switch os.Args[1] {
	case "build-web":
		if err := buildweb.Run(); err != nil {
			exitWithError(err)
		}
	case "ut":
		if err := ut.Run(); err != nil {
			exitWithError(err)
		}
	case "it":
		if err := it.Run(); err != nil {
			exitWithError(err)
		}
	case "e2e":
		os.Exit(e2e.Run(os.Args[2:]))
	case "e2e-install":
		if err := e2e.Install(os.Stdout, os.Stderr); err != nil {
			exitWithError(err)
		}
	default:
		exitWithError(fmt.Errorf("未知脚本命令: %s", os.Args[1]))
	}
}

// exitWithError 输出 err 参数并以失败状态退出。
func exitWithError(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
