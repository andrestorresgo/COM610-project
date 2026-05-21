package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"tracking-api/internal/cache"
)

// OrderReadyEvent represents the event payload received from RabbitMQ
type OrderReadyEvent struct {
	OrderID      int64  `json:"orderId"`
	ClientID     int64  `json:"clientId"`
	CourierID    int64  `json:"courierId"`
	RestaurantID int64  `json:"restaurantId"`
	Timestamp    string `json:"timestamp"`
}

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

	// Inbound order ready events from RabbitMQ.
	OrderReady chan OrderReadyEvent

	// Redis cache store reference
	redisStore *cache.RedisStore
}

// NewHub creates a new connection hub.
func NewHub(redisStore *cache.RedisStore) *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		stop:       make(chan struct{}),
		OrderReady: make(chan OrderReadyEvent, 100),
		redisStore: redisStore,
	}
}

// Run listens on channels for client registration, unregistration, and broadcasting.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			if client.ClientID != "" {
				log.Printf("[Hub] Customer client registered: %s (remote addr: %s)", client.ClientID, client.conn.RemoteAddr())
			} else {
				log.Printf("[Hub] Courier client registered: %s (remote addr: %s)", client.CourierID, client.conn.RemoteAddr())
			}
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				if client.ClientID != "" {
					log.Printf("[Hub] Customer client unregistered: %s", client.ClientID)
				} else {
					log.Printf("[Hub] Courier client unregistered: %s", client.CourierID)
				}
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
		case event := <-h.OrderReady:
			clientIDStr := fmt.Sprintf("%d", event.ClientID)
			found := false
			for client := range h.clients {
				if client.ClientID == clientIDStr {
					found = true
					go h.startCourierTrackingSubscription(client, event)
				}
			}
			if !found {
				log.Printf("[Hub] No active WebSocket connection found for client %s to start subscription", clientIDStr)
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

// startCourierTrackingSubscription polls Redis for the courier's location and sends it to the client
func (h *Hub) startCourierTrackingSubscription(client *Client, event OrderReadyEvent) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	courierIDStr := fmt.Sprintf("%d", event.CourierID)
	log.Printf("[Hub] Starting location subscription for client %s tracking courier %s for order %d", client.ClientID, courierIDStr, event.OrderID)

	for {
		select {
		case <-client.ctx.Done():
			log.Printf("[Hub] Subscription stopped for client %s (disconnected/unregistered)", client.ClientID)
			return
		case <-ticker.C:
			// Fetch courier active time first to check if stale
			ctxTime, cancelTime := context.WithTimeout(context.Background(), 2*time.Second)
			lastActive, errTime := h.redisStore.GetCourierActiveTime(ctxTime, courierIDStr)
			cancelTime()

			isStale := false
			if errTime != nil {
				isStale = true
			} else {
				if time.Now().Unix()-lastActive > 15 {
					isStale = true
				}
			}

			if isStale {
				statusMsg := map[string]interface{}{
					"order_id": event.OrderID,
					"status":   "Reconnecting to Courier...",
				}
				msgBytes, _ := json.Marshal(statusMsg)
				select {
				case client.send <- msgBytes:
				default:
				}
				continue
			}

			// Fetch courier location
			ctxLoc, cancelLoc := context.WithTimeout(context.Background(), 2*time.Second)
			pos, errLoc := h.redisStore.GetCourierLocation(ctxLoc, courierIDStr)
			cancelLoc()

			if errLoc != nil {
				statusMsg := map[string]interface{}{
					"order_id": event.OrderID,
					"status":   "Reconnecting to Courier...",
				}
				msgBytes, _ := json.Marshal(statusMsg)
				select {
				case client.send <- msgBytes:
				default:
				}
				continue
			}

			// Send coordinates
			coordMsg := map[string]interface{}{
				"order_id":  event.OrderID,
				"latitude":  pos.Latitude,
				"longitude": pos.Longitude,
				"status":    "active",
			}
			msgBytes, _ := json.Marshal(coordMsg)
			select {
			case client.send <- msgBytes:
			default:
			}
		}
	}
}
