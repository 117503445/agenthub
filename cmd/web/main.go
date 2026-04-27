package main

import (
	"context"
	"os"

	"github.com/117503445/goutils/glog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/117503445/coding/internal/buildinfo"
	"github.com/117503445/coding/internal/envloader"
)

// init 初始化日志并主动加载 .env 文件。
func init() {
	initLog()
	if err := envloader.LoadDefault(); err != nil {
		log.Warn().Err(err).Msg("加载 .env 失败")
	}
}

// initLog 根据环境变量初始化日志颜色。
func initLog() {
	if os.Getenv("CODING_LOG_NO_COLOR") == "true" {
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

	log.Info().
		Str("BuildTime", buildinfo.BuildTime).
		Str("GitBranch", buildinfo.GitBranch).
		Str("GitCommit", buildinfo.GitCommit).
		Str("GitTag", buildinfo.GitTag).
		Str("GitDirty", buildinfo.GitDirty).
		Str("GitVersion", buildinfo.Version()).
		Str("BuildDir", buildinfo.BuildDir).
		Msg("构建信息")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx := log.Logger.WithContext(context.Background())
	if err := ListenAndServe(ctx, port); err != nil {
		log.Panic().Err(err).Msg("服务启动失败")
	}
}
