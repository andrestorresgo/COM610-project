package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// ANSI terminal color escape codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// GPSPayload matches the server-side internal/ws/client.go structure
type GPSPayload struct {
	OrderID   string  `json:"order_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Predefined route coordinates in Sucre, Bolivia (matching client UI)
var sucreRoute = []struct{ Lat, Lng float64 }{
	{Lat: -19.0429, Lng: -65.2627}, // Restaurant
	{Lat: -19.0435, Lng: -65.2618},
	{Lat: -19.0441, Lng: -65.2609},
	{Lat: -19.0447, Lng: -65.2600},
	{Lat: -19.0453, Lng: -65.2591}, // Midpoint
	{Lat: -19.0459, Lng: -65.2582},
	{Lat: -19.0465, Lng: -65.2573},
	{Lat: -19.0471, Lng: -65.2564},
	{Lat: -19.0477, Lng: -65.2555}, // Near Customer
	{Lat: -19.0483, Lng: -65.2546}, // Customer Address
}

var errRouteCompleted = fmt.Errorf("route completed")

func main() {
	// 1. CLI Flags definition
	wsURLFlag := flag.String("url", "ws://localhost:8080", "WebSocket base URL (e.g. ws://localhost:8080)")
	courierFlag := flag.String("courier", "courier-123", "Courier identifier to register as")
	orderFlag := flag.String("order", "order-456", "Order ID to send location updates for")
	intervalFlag := flag.Duration("interval", 2*time.Second, "Send frequency interval (e.g. 2s, 500ms)")
	loopFlag := flag.Bool("loop", true, "Continuously loop the courier route (reversing direction when reaching endpoints)")
	reconnectFlag := flag.Bool("reconnect", true, "Automatically reconnect if connection drops")

	flag.Parse()

	// Parse and format the target URL
	parsedURL, err := url.Parse(*wsURLFlag)
	if err != nil {
		log.Fatalf("%s[ERROR] Invalid WebSocket URL: %v%s", colorRed, err, colorReset)
	}

	// Ensure correct path and query parameters
	if parsedURL.Path == "" || parsedURL.Path == "/" {
		parsedURL.Path = "/ws/track"
	}
	query := parsedURL.Query()
	query.Set("courier_id", *courierFlag)
	parsedURL.RawQuery = query.Encode()

	// Switch http(s) to ws(s) if provided accidentally
	if parsedURL.Scheme == "http" {
		parsedURL.Scheme = "ws"
	} else if parsedURL.Scheme == "https" {
		parsedURL.Scheme = "wss"
	}

	wsURL := parsedURL.String()

	fmt.Printf("%sTarget URL:%s    %s\n", colorBold, colorReset, wsURL)
	fmt.Printf("%sCourier ID:%s    %s\n", colorBold, colorReset, *courierFlag)
	fmt.Printf("%sOrder ID:%s      %s\n", colorBold, colorReset, *orderFlag)
	fmt.Printf("%sInterval:%s      %v\n", colorBold, colorReset, *intervalFlag)
	fmt.Printf("%sLoop Route:%s    %t (ping-pong directions)\n", colorBold, colorReset, *loopFlag)
	fmt.Printf("%sAuto-Reconnect:%s %t\n", colorBold, colorReset, *reconnectFlag)
	fmt.Printf("%s==================================================%s\n\n", colorBlue, colorReset)

	// Set up OS signal interception for graceful shutdown
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Context to control connection cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS shutdown signal in a goroutine
	go func() {
		sig := <-shutdownChan
		fmt.Printf("\n%s[SHUTDOWN] Received signal %v. Gracefully shutting down...%s\n", colorYellow, sig, colorReset)
		cancel()
	}()

	routeIndex := 0
	forwardDirection := true
	reconnectAttempts := 0

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("%s[EXIT] Simulation ended gracefully.%s\n", colorGreen, colorReset)
			return
		default:
			// Run the WebSocket connection session
			err := runSession(ctx, wsURL, *orderFlag, *intervalFlag, *loopFlag, &routeIndex, &forwardDirection)
			if err == errRouteCompleted {
				fmt.Printf("%s[EXIT] Route completed successfully. Exiting simulation.%s\n", colorGreen, colorReset)
				return
			}
			if err != nil {
				fmt.Printf("%s[DISCONNECT] Connection closed/failed: %v%s\n", colorRed, err, colorReset)
			}

			if ctx.Err() != nil {
				fmt.Printf("%s[EXIT] Simulation ended gracefully.%s\n", colorGreen, colorReset)
				return
			}

			if !*reconnectFlag {
				fmt.Printf("%s[EXIT] Reconnect disabled. Exiting.%s\n", colorYellow, colorReset)
				return
			}

			// Exponential backoff reconnect
			reconnectAttempts++
			backoff := time.Duration(reconnectAttempts) * time.Second
			if backoff > 10*time.Second {
				backoff = 10 * time.Second
			}
			fmt.Printf("%s[RECONNECT] Reconnecting in %v (Attempt %d)...%s\n", colorYellow, backoff, reconnectAttempts, colorReset)

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				// Wait and try again
			}
		}
	}
}

// runSession handles a single WebSocket connection lifecycle
func runSession(
	ctx context.Context,
	wsURL string,
	orderID string,
	interval time.Duration,
	loop bool,
	routeIndex *int,
	forwardDirection *bool,
) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	fmt.Printf("%s[CONNECT] Dialing WebSocket server...%s\n", colorCyan, colorReset)
	conn, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	defer conn.Close()
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	fmt.Printf("%s[SUCCESS] WebSocket connection established successfully!%s\n", colorGreen, colorReset)

	// Channel to signal read error/closure to the main loop
	readErrChan := make(chan error, 1)

	// Start a read pump goroutine to consume incoming messages and handle ping/pong control messages
	go func() {
		defer close(readErrChan)
		for {
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				readErrChan <- err
				return
			}
			if messageType == websocket.TextMessage {
				fmt.Printf("%s[RECEIVE] Received message from server: %s%s\n", colorPurple, string(payload), colorReset)
			}
		}
	}()

	// Loop to send mock GPS data periodic updates
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Cleanly close connection
			fmt.Printf("%s[CLOSE] Sending close frame to server...%s\n", colorYellow, colorReset)
			closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Client shutting down")
			_ = conn.WriteMessage(websocket.CloseMessage, closeMsg)
			// Small sleep to allow server to receive close frame
			time.Sleep(500 * time.Millisecond)
			return nil

		case err := <-readErrChan:
			// Read loop failed (disconnected or error)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return fmt.Errorf("unexpected socket close: %w", err)
			}
			return fmt.Errorf("socket closed: %w", err)

		case <-ticker.C:
			// Get current coordinates
			pos := sucreRoute[*routeIndex]
			payload := GPSPayload{
				OrderID:   orderID,
				Latitude:  pos.Lat,
				Longitude: pos.Lng,
			}

			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				fmt.Printf("%s[ERROR] Failed to marshal GPS payload: %v%s\n", colorRed, err, colorReset)
				continue
			}

			// Send coordinates
			fmt.Printf("%s[SEND] Order: #%s | Lat: %8.5f | Lng: %8.5f (Point %d/%d)%s\n",
				colorGreen, orderID, pos.Lat, pos.Lng, *routeIndex+1, len(sucreRoute), colorReset)

			err = conn.WriteMessage(websocket.TextMessage, payloadBytes)
			if err != nil {
				return fmt.Errorf("failed to write message: %w", err)
			}

			// Advance route pointer
			if *forwardDirection {
				*routeIndex++
				if *routeIndex >= len(sucreRoute) {
					if loop {
						// Reverse direction (go backwards)
						*forwardDirection = false
						*routeIndex = len(sucreRoute) - 2
						fmt.Printf("%s[ROUTE] Destination reached. Reversing direction back to restaurant!%s\n", colorYellow, colorReset)
					} else {
						fmt.Printf("%s[ROUTE] Route completed! Stopping.%s\n", colorGreen, colorReset)
						return errRouteCompleted
					}
				}
			} else {
				*routeIndex--
				if *routeIndex < 0 {
					if loop {
						// Reverse direction (go forwards again)
						*forwardDirection = true
						*routeIndex = 1
						fmt.Printf("%s[ROUTE] Restaurant reached. Starting route again!%s\n", colorYellow, colorReset)
					} else {
						fmt.Printf("%s[ROUTE] Route completed! Stopping.%s\n", colorGreen, colorReset)
						return errRouteCompleted
					}
				}
			}
		}
	}
}
