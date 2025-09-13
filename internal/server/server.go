// internal/server/server.go
package server

import (
    "context"
    "log"
    "net/http"
    "strings"

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
    luaEngine    *lua.Engine // ADD THIS LINE: Reference to the Lua engine
    httpServer   *http.Server
    getBleStatus func() bool
    getBleRssi   func() int16
    getSchedules func() map[cron.EntryID]scheduler.ScheduleEntry

    staticFilesDir string
    allowedOrigins []string
    upgrader       websocket.Upgrader
}

// NewServer creates a new server instance.
// ADD luaEngine *lua.Engine to the function parameters
func NewServer(handler CommandHandler, luaEngine *lua.Engine, getBleStatus func() bool, getBleRssi func() int16, getSchedules func() map[cron.EntryID]scheduler.ScheduleEntry, port string, staticFilesDir string, allowedOrigins []string) *Server {
    hub := NewHub()
    go hub.Run()

    s := &Server{
        Hub:          hub,
        handler:      handler,
        luaEngine:    luaEngine, // ASSIGN THE PASSED luaEngine HERE
        getBleStatus: getBleStatus,
        getBleRssi:   getBleRssi,
        getSchedules: getSchedules,

        staticFilesDir: staticFilesDir,
        allowedOrigins: allowedOrigins,
    }

    // Initialize the upgrader here so it can access s.allowedOrigins
    s.upgrader = websocket.Upgrader{
        CheckOrigin: func(r *http.Request) bool {
            // If allowedOrigins is empty, allow all (for development convenience)
            if len(s.allowedOrigins) == 0 {
                log.Println("Warning: WebSocket CheckOrigin is disabled (allowedOrigins is empty).")
                return true
            }
            origin := r.Header.Get("Origin")
            for _, allowed := range s.allowedOrigins {
                if strings.EqualFold(origin, allowed) { // Case-insensitive comparison
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
    // Use the instance-specific upgrader
    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("WebSocket upgrade error: %v", err)
        return
    }
    defer conn.Close() // Ensure connection is closed on function exit

    // Send RSSI along with connection status.
    initialBleStatus := s.getBleStatus()
    initialBleRssi := s.getBleRssi()
    err = conn.WriteJSON(NewMessage("ble_status", map[string]interface{}{
        "connected": initialBleStatus,
        "rssi":      initialBleRssi,
    }))
    if err != nil {
        log.Printf("Error sending initial ble_status: %v", err)
        return
    }

    // Send initial pattern list.
    // CHANGE THE CALL TO USE THE s.luaEngine field
    patterns, err := s.luaEngine.GetPatternList()
    if err != nil {
        log.Printf("Could not get pattern list for new client: %v", err)
    } else {
        err := conn.WriteJSON(NewMessage("pattern_list", patterns))
        if err != nil {
            log.Printf("Error sending initial pattern_list: %v", err)
            return
        }
    }

    schedules := s.getSchedules()
    err = conn.WriteJSON(NewMessage("schedule_list", schedules))
    if err != nil {
        log.Printf("Error sending initial schedule_list: %v", err)
        return
    }

    s.Hub.register <- conn

    defer func() {
        s.Hub.unregister <- conn
    }()

    for {
        _, msgBytes, err := conn.ReadMessage()
        if err != nil {
            // log.Printf("WebSocket read error (client disconnected?): %v", err) // Too noisy, client disconnect is common
            break
        }
        s.handler.Handle(Message{Raw: msgBytes}, s.Hub)
    }
}
