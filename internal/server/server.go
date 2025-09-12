package server

import (
	"context"
	"log"
	"net/http"

	"bledom-controller/internal/lua"
	"bledom-controller/internal/scheduler"
	"github.com/gorilla/websocket"
	"github.com/robfig/cron/v3"
)

// ClientConn defines an interface for a WebSocket connection.
type ClientConn interface {
	WriteJSON(v interface{}) error
}

// CommandHandler defines the interface for handling client commands.
type CommandHandler interface {
	Handle(msg Message, hub *Hub)
}

// Server manages the HTTP and WebSocket services.
type Server struct {
	Hub          *Hub
	handler      CommandHandler
	httpServer   *http.Server
	getBleStatus func() bool
	getBleRssi   func() int16 // NEW: Function to get initial RSSI
	getSchedules func() map[cron.EntryID]scheduler.ScheduleEntry
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewServer creates a new server instance.
// MODIFIED: NewServer signature updated to accept getBleRssi.
func NewServer(handler CommandHandler, getBleStatus func() bool, getBleRssi func() int16, getSchedules func() map[cron.EntryID]scheduler.ScheduleEntry) *Server {
	hub := NewHub()
	go hub.Run()

	s := &Server{
		Hub:          hub,
		handler:      handler,
		getBleStatus: getBleStatus,
		getBleRssi:   getBleRssi, // NEW: Store the function
		getSchedules: getSchedules,
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("./static")))
	mux.HandleFunc("/ws", s.handleWebSocket)
	s.httpServer = &http.Server{Addr: ":8080", Handler: mux}

	return s
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// handleWebSocket now safely handles sending initial state.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Send RSSI along with connection status.
	initialBleStatus := s.getBleStatus()
	initialBleRssi := s.getBleRssi()
	err = conn.WriteJSON(NewMessage("ble_status", map[string]interface{}{
		"connected": initialBleStatus,
		"rssi":      initialBleRssi,
	}))
	if err != nil {
		log.Printf("Error sending initial ble_status: %v", err)
		conn.Close()
		return
	}

	// Send initial pattern list.
	patterns, err := lua.GetPatternList()
	if err != nil {
		log.Printf("Could not get pattern list for new client: %v", err)
	} else {
		err := conn.WriteJSON(NewMessage("pattern_list", patterns))
		if err != nil {
			log.Printf("Error sending initial pattern_list: %v", err)
			conn.Close()
			return
		}
	}

	schedules := s.getSchedules()
	err = conn.WriteJSON(NewMessage("schedule_list", schedules))
	if err != nil {
		log.Printf("Error sending initial schedule_list: %v", err)
		conn.Close()
		return
	}

	s.Hub.register <- conn

	defer func() {
		s.Hub.unregister <- conn
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break // Client disconnected
		}
		s.handler.Handle(Message{Raw: msgBytes}, s.Hub)
	}
}
