package server

// Command represents an incoming JSON command from a WebSocket client.
type Command struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// Message represents an outgoing JSON message sent to WebSocket clients.
type Message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	Raw     []byte      `json:"-"` // Used for raw message handling if needed
}

// NewMessage creates a new structured Message for broadcasting to clients.
func NewMessage(msgType string, payload interface{}) Message {
	return Message{Type: msgType, Payload: payload}
}
