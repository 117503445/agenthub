package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"path"
	"strings"
)

// tokenAuth 处理前端访问 token 鉴权。
type tokenAuth struct {
	token string // token 表示服务端配置的访问 token。
}

// authStatusResponse 表示前端鉴权状态响应。
type authStatusResponse struct {
	TokenRequired bool `json:"tokenRequired"` // TokenRequired 表示前端是否必须输入 token。
}

// newTokenAuth 使用 token 参数创建 tokenAuth。
func newTokenAuth(token string) tokenAuth {
	return tokenAuth{token: strings.TrimSpace(token)}
}

// Required 返回当前服务是否要求 token。
func (a tokenAuth) Required() bool {
	return a.token != ""
}

// ServeStatus 使用 w 和 r 参数返回前端鉴权状态。
func (a tokenAuth) ServeStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(authStatusResponse{
		TokenRequired: a.Required(),
	})
}

// ServeWS 使用 w、r 和 next 参数校验 token 后继续处理 WebSocket。
func (a tokenAuth) ServeWS(w http.ResponseWriter, r *http.Request, next func(http.ResponseWriter, *http.Request)) {
	if !a.Valid(r) {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	next(w, r)
}

// Valid 使用 r 参数判断请求是否携带正确 token。
func (a tokenAuth) Valid(r *http.Request) bool {
	if !a.Required() {
		return true
	}
	got := r.URL.Query().Get("token")
	if got == "" || len(got) != len(a.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) == 1
}

// isAuthStatusPath 使用 requestPath 参数判断是否为鉴权状态路径。
func isAuthStatusPath(requestPath string) bool {
	cleanPath := path.Clean(requestPath)
	return cleanPath == "/auth/status" || strings.HasSuffix(cleanPath, "/auth/status")
}
