package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"bledom-controller/internal/core"
	"bledom-controller/internal/lua"

	"github.com/gorilla/websocket"
)

// ClientConn defines an interface for a WebSocket connection.
type ClientConn interface {
	WriteJSON(v interface{}) error
}

// Command is the raw JSON structure from websockets
type incomingCommand struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// Server manages the HTTP and WebSocket services.
type Server struct {
	Hub        *Hub
	luaEngine  *lua.Engine
	httpServer *http.Server

	eventBus       *core.EventBus
	commandChannel core.CommandChannel
	state          *core.State

	staticFilesDir string
	allowedOrigins []string
	upgrader       websocket.Upgrader
}

// NewServer creates a new server instance.
func NewServer(luaEngine *lua.Engine, eb *core.EventBus, st *core.State, cmdChan core.CommandChannel, port string, staticFilesDir string, allowedOrigins []string) *Server {
	hub := NewHub()
	go hub.Run()

	// Create new server instance
	s := &Server{
		Hub:            hub,
		luaEngine:      luaEngine,
		eventBus:       eb,
		state:          st,
		commandChannel: cmdChan,

		staticFilesDir: staticFilesDir,
		allowedOrigins: allowedOrigins,
	}

	// Initialize WebSocket upgrader
	s.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if len(s.allowedOrigins) == 0 {
				log.Println("Warning: WebSocket CheckOrigin is disabled.")
				return true
			}
			origin := r.Header.Get("Origin")
			for _, allowed := range s.allowedOrigins {
				if strings.EqualFold(origin, allowed) {
					return true
				}
			}
			log.Printf("WebSocket connection blocked: Origin '%s' not in allowed list.", origin)
			return false
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(s.staticFilesDir)))
	mux.HandleFunc("/ws", s.handleWebSocket)
	s.httpServer = &http.Server{Addr: ":" + port, Handler: mux}

	// Subscribe to events to broadcast via WS
	go s.listenEvents()

	return s
}

// listenEvents subscribes to events from the event bus and broadcasts them to WebSocket clients.
func (s *Server) listenEvents() {
	if s.eventBus == nil {
		return
	}

	sub := s.eventBus.Subscribe(
		core.StateChangedEvent,
		core.DeviceConnectedEvent,
		core.PatternChangedEvent,
		core.PowerChangedEvent,
		core.ColorChangedEvent,
	)

	for event := range sub {
		switch event.Type {
		case core.DeviceConnectedEvent:
			s.Hub.Broadcast(NewMessage("ble_status", event.Payload))
		case core.StateChangedEvent:
			s.Hub.Broadcast(NewMessage("device_state", event.Payload))
		case core.PatternChangedEvent:
			s.Hub.Broadcast(NewMessage("pattern_status", event.Payload))
		case core.PowerChangedEvent:
			s.Hub.Broadcast(NewMessage("power_update", event.Payload))
		case core.ColorChangedEvent:
			s.Hub.Broadcast(NewMessage("color_update", event.Payload))
		}
	}
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	if s.state != nil {
		st := s.state.Clone()

		// Send BLE status
		_ = conn.WriteJSON(NewMessage("ble_status", map[string]interface{}{
			"connected": st.IsConnected,
			"rssi":      st.RSSI,
		}))

		// Send current device state
		hex := fmt.Sprintf("#%02X%02X%02X", st.ColorR, st.ColorG, st.ColorB)
		_ = conn.WriteJSON(NewMessage("device_state", map[string]interface{}{
			"isOn":       st.Power,
			"r":          st.ColorR,
			"g":          st.ColorG,
			"b":          st.ColorB,
			"hex":        hex,
			"brightness": st.Brightness,
			"speed":      st.Speed,
		}))

		// Send current running pattern
		_ = conn.WriteJSON(NewMessage("pattern_status", map[string]interface{}{
			"running": st.RunningPattern,
		}))
	}

	// Send pattern list
	patterns, err := s.luaEngine.GetPatternList()
	if err == nil {
		_ = conn.WriteJSON(NewMessage("pattern_list", patterns))
	}

	s.Hub.register <- conn

	defer func() {
		s.Hub.unregister <- conn
	}()

	for {
		// Read incoming command from websocket
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		// Incoming command from websocket
		var rawCmd incomingCommand
		if err := json.Unmarshal(msgBytes, &rawCmd); err != nil {
			log.Printf("Error unmarshalling raw command: %v", err)
			continue
		}

		// Convert JSON unmarshalled standard command from websocket payload into internal Command
		cmd := core.Command{
			Type:    core.CommandType(rawCmd.Type),
			Payload: rawCmd.Payload,
		}

		// Send command to command channel
		if s.commandChannel != nil {
			s.commandChannel <- cmd
		}
	}
}
