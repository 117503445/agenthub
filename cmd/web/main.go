package main

import (
	"context"
	"fmt"
	"os"

	"github.com/117503445/goutils/glog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/117503445/agenthub/internal/buildinfo"
	"github.com/117503445/agenthub/internal/envloader"
)

// initLog 使用 noColor 参数初始化日志颜色。
func initLog(noColor bool) {
	if noColor {
		logger := log.Output(glog.NewConsoleWriter(glog.ConsoleWriterConfig{
			NoColor: true,
		})).Level(zerolog.DebugLevel).With().Caller().Logger()
		glog.InitZeroLog(glog.InitZeroLogConfig{Logger: &logger})
		return
	}
	glog.InitZeroLog()
}

// main 读取端口配置并启动 WebSocket Web 服务。
func main() {
	if runMockAgentCLIIfRequested(os.Args) {
		return
	}

	envErr := envloader.LoadDefault()
	config, err := parseWebConfig(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	initLog(config.LogNoColor)
	if envErr != nil {
		log.Warn().Err(envErr).Msg("加载 .env 失败")
	}

	log.Info().
		Str("BuildTime", buildinfo.BuildTime).
		Str("GitBranch", buildinfo.GitBranch).
		Str("GitCommit", buildinfo.GitCommit).
		Str("GitTag", buildinfo.GitTag).
		Str("GitDirty", buildinfo.GitDirty).
		Str("GitVersion", buildinfo.Version()).
		Str("BuildDir", buildinfo.BuildDir).
		Msg("构建信息")

	ctx := log.Logger.WithContext(context.Background())
	if err := ListenAndServe(ctx, config); err != nil {
		log.Panic().Err(err).Msg("服务启动失败")
	}
}
