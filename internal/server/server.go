// Package server provides HTTP and WebSocket services for the BLEDOM controller.
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
	"bledom-controller/internal/scheduler"

	"github.com/gorilla/websocket"
)

// ClientConn defines an interface for a WebSocket connection, facilitating testing.
type ClientConn interface {
	WriteJSON(v interface{}) error
}

// incomingCommand represents the raw JSON structure of commands received via WebSockets.
type incomingCommand struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// Server manages the HTTP and WebSocket endpoints and handles client coordination.
type Server struct {
	Hub        *Hub
	luaEngine  *lua.Engine
	httpServer *http.Server

	eventBus       *core.EventBus
	commandChannel core.CommandChannel
	state          *core.State
	scheduler      *scheduler.Scheduler

	webFilesDir    string
	allowedOrigins []string
	upgrader       websocket.Upgrader
}

// NewServer creates and initializes a new Server instance.
func NewServer(luaEngine *lua.Engine, eb *core.EventBus, st *core.State, sched *scheduler.Scheduler, cmdChan core.CommandChannel, port string, webFilesDir string, allowedOrigins []string) *Server {
	hub := NewHub()
	go hub.Run()

	s := &Server{
		Hub:            hub,
		luaEngine:      luaEngine,
		eventBus:       eb,
		state:          st,
		scheduler:      sched,
		commandChannel: cmdChan,

		webFilesDir:    webFilesDir,
		allowedOrigins: allowedOrigins,
	}

	// Initialize WebSocket upgrader with standard buffers
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  512,
		WriteBufferSize: 512,
		CheckOrigin: func(r *http.Request) bool {
			if len(s.allowedOrigins) == 0 {
				log.Println("[Server] Warning: WebSocket CheckOrigin is disabled (allowing all).")
				return true
			}
			origin := r.Header.Get("Origin")
			for _, allowed := range s.allowedOrigins {
				if strings.EqualFold(origin, allowed) {
					return true
				}
			}
			log.Printf("[Server] WebSocket connection blocked: Origin '%s' not in allowed list.", origin)
			return false
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(s.webFilesDir)))
	mux.HandleFunc("/ws", s.handleWebSocket)
	s.httpServer = &http.Server{Addr: ":" + port, Handler: mux}

	// Subscribe to internal events to broadcast them to clients
	go s.listenEvents()

	return s
}

// listenEvents subscribes to the event bus and broadcasts relevant events to all connected WebSocket clients.
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

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleWebSocket upgrades HTTP connections to WebSocket and handles client communication.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Server] WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	if s.state != nil {
		st := s.state.Clone()

		// Send initial BLE connection status
		_ = conn.WriteJSON(NewMessage("ble_status", map[string]interface{}{
			"connected": st.IsConnected,
			"rssi":      st.RSSI,
		}))

		// Send initial device state
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

		// Send initial running pattern
		_ = conn.WriteJSON(NewMessage("pattern_status", map[string]interface{}{
			"running": st.RunningPattern,
		}))
	}

	// Send available pattern list
	patterns, err := s.luaEngine.GetPatternList()
	if err == nil {
		_ = conn.WriteJSON(NewMessage("pattern_list", patterns))
	}

	// Send current schedule list
	if s.scheduler != nil {
		_ = conn.WriteJSON(NewMessage("schedule_list", s.scheduler.GetAll()))
	}

	s.Hub.register <- conn

	defer func() {
		s.Hub.unregister <- conn
	}()

	for {
		// Read incoming message from client
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var rawCmd incomingCommand
		if err := json.Unmarshal(msgBytes, &rawCmd); err != nil {
			log.Printf("[Server] Error unmarshalling client command: %v", err)
			continue
		}

		// Wrap as internal command and send to orchestrator
		cmd := core.Command{
			Type:    core.CommandType(rawCmd.Type),
			Payload: rawCmd.Payload,
		}

		if s.commandChannel != nil {
			s.commandChannel <- cmd
		}
	}
}
