package main

import (
	"context"
	"os"

	"github.com/117503445/goutils/glog"
	"github.com/rs/zerolog/log"

	"github.com/117503445/coding/internal/buildinfo"
	"github.com/117503445/coding/internal/envloader"
)

// init 初始化日志并主动加载 .env 文件。
func init() {
	glog.InitZeroLog()
	if err := envloader.LoadDefault(); err != nil {
		log.Warn().Err(err).Msg("加载 .env 失败")
	}
}

// main 读取端口配置并启动 WebSocket Web 服务。
func main() {
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
