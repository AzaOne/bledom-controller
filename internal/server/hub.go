package server

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	clients    map[*websocket.Conn]bool
	mu         sync.Mutex
	broadcast  chan Message
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
}

// NewHub initializes and returns a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan Message),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

// Run is the main event loop for the Hub, managing client registration and message broadcasting.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Println("[Server] WebSocket client connected.")
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
				log.Println("[Server] WebSocket client disconnected.")
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				if err := client.WriteJSON(message); err != nil {
					log.Printf("[Server] WebSocket broadcast error: %v", err)
					client.Close()
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

// Broadcast enqueues a message for delivery to all connected WebSocket clients.
func (h *Hub) Broadcast(msg Message) {
	h.broadcast <- msg
}
