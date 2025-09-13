package server

// Command is an incoming message from the WebSocket client.
type Command struct {
    Type    string                 `json:"type"`
    Payload map[string]interface{} `json:"payload"`
}

// Message is an outgoing message to the WebSocket client.
type Message struct {
    Type    string      `json:"type"`
    Payload interface{} `json:"payload"`
    Raw     []byte      `json:"-"` // Used for raw message handling
}

// NewMessage creates a new structured message for broadcasting.
func NewMessage(msgType string, payload interface{}) Message {
    return Message{Type: msgType, Payload: payload}
}
