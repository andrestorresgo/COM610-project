package ws

import (
	"log"
)

// Hub maintains the set of active clients and handles registration/unregistration.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Stop signal channel for graceful shutdown.
	stop chan struct{}
}

// NewHub creates a new connection hub.
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		stop:       make(chan struct{}),
	}
}

// Run listens on channels for client registration, unregistration, and broadcasting.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			log.Printf("[Hub] Courier client registered: %s (remote addr: %s)", client.CourierID, client.conn.RemoteAddr())
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Printf("[Hub] Courier client unregistered: %s", client.CourierID)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		case <-h.stop:
			log.Println("[Hub] Shutting down and closing all active WebSocket connections...")
			for client := range h.clients {
				client.Close()
				delete(h.clients, client)
			}
			return
		}
	}
}

// Stop requests the hub to stop running and cleanly close all connections.
func (h *Hub) Stop() {
	close(h.stop)
}
