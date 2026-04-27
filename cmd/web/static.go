package main

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var staticFS embed.FS

// staticHandler 返回用于服务前端静态资源和 SPA 回退的处理器。
func staticHandler() http.Handler {
	distFS, err := fs.Sub(staticFS, "dist")
	if err != nil {
		panic(err)
	}

	indexHTML, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		panic(err)
	}

	serveIndex := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	}
	// serveFile 使用 w、r 和 filePath 参数返回嵌入的静态文件。
	serveFile := func(w http.ResponseWriter, r *http.Request, filePath string) bool {
		data, err := fs.ReadFile(distFS, filePath)
		if err != nil {
			return false
		}
		modTime := time.Time{}
		if stat, statErr := fs.Stat(distFS, filePath); statErr == nil {
			modTime = stat.ModTime()
		}
		http.ServeContent(w, r, path.Base(filePath), modTime, bytes.NewReader(data))
		return true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := path.Clean(r.URL.Path)
		if requestPath == "/" || requestPath == "." {
			serveIndex(w)
			return
		}

		filePath := requestPath[1:]
		if filePath == "index.html" {
			serveIndex(w)
			return
		}

		if serveFile(w, r, filePath) {
			return
		}
		if assetsIndex := strings.Index(filePath, "assets/"); assetsIndex >= 0 {
			if serveFile(w, r, filePath[assetsIndex:]) {
				return
			}
		}

		serveIndex(w)
	})
}
