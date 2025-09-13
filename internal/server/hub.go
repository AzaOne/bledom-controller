package server

import (
    "log"
    "sync"

    "github.com/gorilla/websocket"
)

// Hub manages WebSocket clients.
type Hub struct {
    clients    map[*websocket.Conn]bool
    mu         sync.Mutex
    broadcast  chan Message
    register   chan *websocket.Conn
    unregister chan *websocket.Conn
}

// NewHub creates a new Hub.
func NewHub() *Hub {
    return &Hub{
        clients:    make(map[*websocket.Conn]bool),
        broadcast:  make(chan Message),
        register:   make(chan *websocket.Conn),
        unregister: make(chan *websocket.Conn),
    }
}

// Run starts the hub's event loop.
func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.mu.Lock()
            h.clients[client] = true
            h.mu.Unlock()
            log.Println("WebSocket client connected.")
        case client := <-h.unregister:
            h.mu.Lock()
            if _, ok := h.clients[client]; ok {
                delete(h.clients, client)
                client.Close()
                log.Println("WebSocket client disconnected.")
            }
            h.mu.Unlock()
        case message := <-h.broadcast:
            h.mu.Lock()
            for client := range h.clients {
                if err := client.WriteJSON(message); err != nil {
                    log.Printf("broadcast error: %v", err)
                    client.Close()
                    delete(h.clients, client)
                }
            }
            h.mu.Unlock()
        }
    }
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(msg Message) {
    h.broadcast <- msg
}
