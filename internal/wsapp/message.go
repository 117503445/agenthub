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
	BuildTime  string `json:"buildTime"`         // BuildTime 表示后端构建时间。
	Hostname   string `json:"hostname"`          // Hostname 表示后端机器名。
}

// ProjectMutationPayload 表示 project 创建和更新消息的参数。
type ProjectMutationPayload struct {
	ID   string `json:"id,omitempty"` // ID 表示要更新的 project 标识。
	Path string `json:"path"`         // Path 表示 project 在后端机器上的工作目录。
}

// ProjectReorderPayload 表示 Project 侧栏重排序请求参数。
type ProjectReorderPayload struct {
	ProjectIDs []string `json:"projectIds"` // ProjectIDs 表示排序后的 project 标识列表。
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
	ChatID   string                `json:"chatId"`           // ChatID 表示目标聊天页标识。
	Prompt   string                `json:"prompt"`           // Prompt 表示用户输入内容。
	Images   []MessageImagePayload `json:"images,omitempty"` // Images 表示随 prompt 发送的图片附件。
	PlanMode bool                  `json:"planMode"`         // PlanMode 表示本轮是否只生成 plan。
}

// ChatStopPayload 表示停止聊天页当前输出的请求参数。
type ChatStopPayload struct {
	ChatID string `json:"chatId"` // ChatID 表示目标聊天页标识。
}

// ChatDetailGetPayload 表示读取聊天页详情的请求参数。
type ChatDetailGetPayload struct {
	ChatID string `json:"chatId"` // ChatID 表示目标聊天页标识。
}

// ChatDraftUpdatePayload 表示更新聊天输入框文字草稿的请求参数。
type ChatDraftUpdatePayload struct {
	ChatID string `json:"chatId"` // ChatID 表示目标聊天页标识。
	Text   string `json:"text"`   // Text 表示尚未发送的输入框文字。
}

// ChatPlanExecutePayload 表示执行已确认 plan 的请求参数。
type ChatPlanExecutePayload struct {
	ChatID string `json:"chatId"` // ChatID 表示目标聊天页标识。
	PlanID string `json:"planId"` // PlanID 表示要执行的 plan 标识。
}

// MessageImagePayload 表示浏览器发送的图片附件。
type MessageImagePayload struct {
	ID       string `json:"id"`       // ID 表示前端生成的附件标识。
	FileName string `json:"fileName"` // FileName 表示图片文件名。
	MimeType string `json:"mimeType"` // MimeType 表示图片 MIME 类型。
	Data     string `json:"data"`     // Data 表示图片 base64 内容。
}

// ChatAgentUpdatePayload 表示更新聊天页 agent 配置的请求参数。
type ChatAgentUpdatePayload struct {
	ChatID    string `json:"chatId"`    // ChatID 表示目标聊天页标识。
	Provider  string `json:"provider"`  // Provider 表示 agent provider。
	Model     string `json:"model"`     // Model 表示 agent 模型。
	Reasoning string `json:"reasoning"` // Reasoning 表示 agent 推理级别。
}

// AgentModelAddPayload 表示新增 agent 模型选项的请求参数。
type AgentModelAddPayload struct {
	Provider string `json:"provider"` // Provider 表示目标 agent provider。
	ID       string `json:"id"`       // ID 表示模型标识。
	Label    string `json:"label"`    // Label 为旧请求兼容字段，服务端忽略并使用 ID。
}

// AgentBuiltinProfilePayload 表示新增内置 Profile 的请求参数。
type AgentBuiltinProfilePayload struct {
	Kind string `json:"kind"` // Kind 表示内置 Profile 类型。
}

// AgentProfileModelPayload 表示 Profile 模型增删改请求参数。
type AgentProfileModelPayload struct {
	ProfileID string `json:"profileId"` // ProfileID 表示目标 Profile 标识。
	ID        string `json:"id"`        // ID 表示模型标识。
	Label     string `json:"label"`     // Label 为旧请求兼容字段，服务端忽略并使用 ID。
	Default   bool   `json:"default"`   // Default 表示是否设为默认模型。
}
