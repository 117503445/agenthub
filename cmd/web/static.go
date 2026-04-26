package main

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
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

	fileServer := http.FileServer(http.FS(distFS))
	serveIndex := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
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

		if file, err := distFS.Open(filePath); err == nil {
			defer file.Close()
			if stat, statErr := file.Stat(); statErr == nil && !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		serveIndex(w)
	})
}
