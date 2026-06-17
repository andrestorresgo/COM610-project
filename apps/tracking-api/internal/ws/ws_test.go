package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"tracking-api/internal/cache"
)

func TestServeWs(t *testing.T) {
	// Use REDIS_URL env var so this test works both inside the Docker network
	// (where Redis is at redis:6379) and on a bare host (localhost:6379).
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	redisStore, err := cache.NewRedisStore(redisURL)
	if err != nil {
		t.Skipf("Skipping integration test: Redis not accessible on localhost:6379: %v", err)
	}

	hub := NewHub(redisStore)
	go hub.Run()
	defer hub.Stop()

	// Start test HTTP server with WS handler
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, redisStore, w, r)
	}))
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?courier_id=test-courier-123"

	// Connect websocket client
	dialer := websocket.Dialer{}
	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to dial websocket: %v", err)
	}
	defer conn.Close()
	resp.Body.Close()

	// Send GPS coordinates message
	payload := GPSPayload{
		OrderID:   "test-order-999",
		Latitude:  -19.0333,
		Longitude: -65.2592,
	}

	msgBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	err = conn.WriteMessage(websocket.TextMessage, msgBytes)
	if err != nil {
		t.Fatalf("Failed to write websocket message: %v", err)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Verify that GPS was saved in Redis under delivery:test-order-999:location
	ctx := context.Background()
	key := "delivery:test-order-999:location"
	pos, err := redisStore.Client.GeoPos(ctx, key, "courier").Result()
	if err != nil {
		t.Fatalf("Failed to get GeoPos from Redis: %v", err)
	}

	if len(pos) == 0 || pos[0] == nil {
		t.Fatalf("No position stored in Redis for key %s", key)
	}

	// Assert position values match with reasonable precision
	lonDiff := pos[0].Longitude - payload.Longitude
	latDiff := pos[0].Latitude - payload.Latitude
	if lonDiff < -0.01 || lonDiff > 0.01 || latDiff < -0.01 || latDiff > 0.01 {
		t.Errorf("Stored location (%f, %f) differs from sent location (%f, %f)",
			pos[0].Longitude, pos[0].Latitude, payload.Longitude, payload.Latitude)
	}

	// Clean up test key from Redis
	redisStore.Client.Del(ctx, key)
}
