package wsapp

type ClientMessage struct {
	Type    string `json:"type"`
	Payload string `json:"payload,omitempty"`
}

type ServerMessage struct {
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload,omitempty"`
	ServerTime string         `json:"serverTime"`
	Version    string         `json:"version"`
}
