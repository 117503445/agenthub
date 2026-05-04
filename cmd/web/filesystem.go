package main

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const filesystemContentRoute = "/fs/content"

// serveFilesystemContent 使用 w 和 r 参数返回后端文件系统中的文件内容。
func serveFilesystemContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if filePath == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	cleanPath := filepath.Clean(filePath)
	if !filepath.IsAbs(cleanPath) {
		http.Error(w, "path must be absolute", http.StatusBadRequest)
		return
	}

	file, err := os.Open(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		if os.IsPermission(err) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		http.Error(w, "open file failed", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "stat file failed", http.StatusInternalServerError)
		return
	}
	if stat.IsDir() {
		http.Error(w, "directory is not supported", http.StatusBadRequest)
		return
	}

	http.ServeContent(w, r, filepath.Base(cleanPath), stat.ModTime(), file)
}

// isFilesystemContentPath 使用 requestPath 参数判断是否为文件系统内容接口路径。
func isFilesystemContentPath(requestPath string) bool {
	cleanPath := path.Clean(requestPath)
	return cleanPath == filesystemContentRoute || strings.HasSuffix(cleanPath, filesystemContentRoute)
}
