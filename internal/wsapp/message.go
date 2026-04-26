package wsapp

// ClientMessage 表示浏览器发送到 WebSocket 服务端的消息。
type ClientMessage struct {
	Type    string `json:"type"`              // Type 表示消息类型。
	Payload string `json:"payload,omitempty"` // Payload 表示消息正文。
}

// ServerMessage 表示 WebSocket 服务端返回给浏览器的消息。
type ServerMessage struct {
	Type       string         `json:"type"`              // Type 表示消息类型。
	Payload    map[string]any `json:"payload,omitempty"` // Payload 表示响应数据。
	ServerTime string         `json:"serverTime"`        // ServerTime 表示服务端发送时间。
	Version    string         `json:"version"`           // Version 表示当前构建版本。
}
