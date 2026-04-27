package wsapp

import "encoding/json"

// ClientMessage 表示浏览器发送到 WebSocket 服务端的消息。
type ClientMessage struct {
	Type    string          `json:"type"`              // Type 表示消息类型。
	Payload json.RawMessage `json:"payload,omitempty"` // Payload 表示消息正文。
}

// ServerMessage 表示 WebSocket 服务端返回给浏览器的消息。
type ServerMessage struct {
	Type       string `json:"type"`              // Type 表示消息类型。
	Payload    any    `json:"payload,omitempty"` // Payload 表示响应数据。
	ServerTime string `json:"serverTime"`        // ServerTime 表示服务端发送时间。
	Version    string `json:"version"`           // Version 表示当前构建版本。
}

// ProjectMutationPayload 表示 project 创建和更新消息的参数。
type ProjectMutationPayload struct {
	ID   string `json:"id,omitempty"` // ID 表示要更新的 project 标识。
	Name string `json:"name"`         // Name 表示 project 展示名称。
	Path string `json:"path"`         // Path 表示 project 在后端机器上的工作目录。
}

// IDPayload 表示只携带 id 参数的请求。
type IDPayload struct {
	ID string `json:"id"` // ID 表示目标资源标识。
}

// ChatCreatePayload 表示创建聊天页的请求参数。
type ChatCreatePayload struct {
	ProjectID string `json:"projectId"` // ProjectID 表示聊天页归属的 project 标识。
}

// ChatSendPayload 表示向聊天页发送 prompt 的请求参数。
type ChatSendPayload struct {
	ChatID string `json:"chatId"` // ChatID 表示目标聊天页标识。
	Prompt string `json:"prompt"` // Prompt 表示用户输入内容。
}

// ChatStopPayload 表示停止聊天页当前输出的请求参数。
type ChatStopPayload struct {
	ChatID string `json:"chatId"` // ChatID 表示目标聊天页标识。
}
