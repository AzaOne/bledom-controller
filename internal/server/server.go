package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"bledom-controller/internal/ble"
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
	Hub               *Hub
	handler           CommandHandler
	luaEngine         *lua.Engine
	httpServer        *http.Server
	getBleStatus      func() bool
	getBleRssi        func() int16
	getSchedules      func() map[cron.EntryID]scheduler.ScheduleEntry
	getRunningPattern func() string
	getDeviceState    func() ble.State // <-- NEW Callback

	staticFilesDir string
	allowedOrigins []string
	upgrader       websocket.Upgrader
}

// NewServer creates a new server instance.
func NewServer(luaEngine *lua.Engine, getBleStatus func() bool, getBleRssi func() int16, getSchedules func() map[cron.EntryID]scheduler.ScheduleEntry, getRunningPattern func() string, getDeviceState func() ble.State, port string, staticFilesDir string, allowedOrigins []string) *Server {
	hub := NewHub()
	go hub.Run()

	s := &Server{
		Hub:               hub,
		handler:           nil,
		luaEngine:         luaEngine,
		getBleStatus:      getBleStatus,
		getBleRssi:        getBleRssi,
		getSchedules:      getSchedules,
		getRunningPattern: getRunningPattern,
		getDeviceState:    getDeviceState, // <-- Assign callback

		staticFilesDir: staticFilesDir,
		allowedOrigins: allowedOrigins,
	}

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

	return s
}

// SetHandler встановлює обробник команд.
func (s *Server) SetHandler(h CommandHandler) {
	s.handler = h
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

	// 1. Відправка статусу BLE
	initialBleStatus := s.getBleStatus()
	initialBleRssi := s.getBleRssi()
	_ = conn.WriteJSON(NewMessage("ble_status", map[string]interface{}{
		"connected": initialBleStatus,
		"rssi":      initialBleRssi,
	}))

	// 2. Відправка поточного стану пристрою (для UI)
	if s.getDeviceState != nil {
		state := s.getDeviceState()
		hex := fmt.Sprintf("#%02X%02X%02X", state.R, state.G, state.B)
		_ = conn.WriteJSON(NewMessage("device_state", map[string]interface{}{
			"isOn":       state.IsOn,
			"r":          state.R,
			"g":          state.G,
			"b":          state.B,
			"hex":        hex,
			"brightness": state.Brightness,
			"speed":      state.Speed,
		}))
	}

	// 3. Відправка списку патернів
	patterns, err := s.luaEngine.GetPatternList()
	if err == nil {
		_ = conn.WriteJSON(NewMessage("pattern_list", patterns))
	}

	// 4. Відправка поточного запущеного патерну
	runningPattern := ""
	if s.getRunningPattern != nil {
		runningPattern = s.getRunningPattern()
	}
	_ = conn.WriteJSON(NewMessage("pattern_status", map[string]string{
		"running": runningPattern,
	}))

	// 5. Відправка розкладу
	schedules := s.getSchedules()
	_ = conn.WriteJSON(NewMessage("schedule_list", schedules))

	s.Hub.register <- conn

	defer func() {
		s.Hub.unregister <- conn
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if s.handler != nil {
			s.handler.Handle(Message{Raw: msgBytes}, s.Hub)
		}
	}
}
