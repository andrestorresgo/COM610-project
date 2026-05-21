package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"tracking-api/internal/cache"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = 30 * time.Second

	// Maximum message size allowed from peer.
	maxMessageSize = 1024
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all in development (default behavior if origin is not verified, or check environment)
		allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
		if allowedOrigin == "" || allowedOrigin == "*" {
			return true
		}
		origin := r.Header.Get("Origin")
		return origin == allowedOrigin
	},
}

// GPSPayload represents the high-frequency coordinate updates sent by the courier.
type GPSPayload struct {
	OrderID   string  `json:"order_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	// Courier identifier
	CourierID string

	// Client identifier (for customers tracking orders)
	ClientID string

	// Redis cache store reference to save location updates
	redisStore *cache.RedisStore

	// context and cancel for tracking client connection lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// ReadPump pumps messages from the websocket connection to the hub/cache.
//
// The application runs ReadPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) ReadPump() {
	defer func() {
		c.cancel()
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Client] Read error: %v", err)
			}
			break
		}

		var payload GPSPayload
		if err := json.Unmarshal(message, &payload); err != nil {
			log.Printf("[Client] Failed to unmarshal message: %v", err)
			continue
		}

		if payload.OrderID == "" {
			log.Printf("[Client] Missing order_id in GPS payload")
			continue
		}

		// Save the coordinates to Redis using GEOADD under delivery:{order_id}:location
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = c.redisStore.UpdateDeliveryLocation(ctx, payload.OrderID, payload.Longitude, payload.Latitude)
		cancel()

		if err != nil {
			log.Printf("[Client] Failed to save GPS coordinates to Redis: %v", err)
		} else {
			log.Printf("[Client] Saved location for order %s: (%f, %f)", payload.OrderID, payload.Longitude, payload.Latitude)
		}

		// Also update the courier location in activeCouriersKey and their active timestamp
		if c.CourierID != "" && c.CourierID != "unknown_courier" {
			ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
			_ = c.redisStore.UpdateCourierLocation(ctx2, c.CourierID, payload.Longitude, payload.Latitude)
			_ = c.redisStore.UpdateCourierActiveTime(ctx2, c.CourierID)
			cancel2()
		}
	}
}

// WritePump pumps messages from the hub to the websocket connection.
//
// A goroutine running WritePump is started for each connection. The
// application ensures that there is at most one writer on a connection by
// executing all writes from this goroutine.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			// Add queued messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				_, _ = w.Write([]byte{'\n'})
				_, _ = w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Close cleanly closes the WebSocket connection with a shutdown message.
func (c *Client) Close() {
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server is shutting down"))
	_ = c.conn.Close()
}

// ServeWs handles websocket requests from the peer.
func ServeWs(hub *Hub, redisStore *cache.RedisStore, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}

	courierID := r.URL.Query().Get("courier_id")
	clientID := r.URL.Query().Get("client_id")
	if courierID == "" && clientID == "" {
		courierID = "unknown_courier"
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		hub:        hub,
		conn:       conn,
		send:       make(chan []byte, 256),
		CourierID:  courierID,
		ClientID:   clientID,
		redisStore: redisStore,
		ctx:        ctx,
		cancel:     cancel,
	}
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.WritePump()
	go client.ReadPump()
}
