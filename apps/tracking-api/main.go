package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Tracking struct {
	OrderID   string `json:"order_id"`
	Status    string `json:"status"`
	Location  string `json:"location"`
	Timestamp string `json:"timestamp"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"service":   "tracking-api",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func main() {
	http.HandleFunc("/health", healthHandler)

	log.Println("Tracking API starting on :3002...")
	if err := http.ListenAndServe(":3002", nil); err != nil {
		log.Fatalf("Could not start server: %v\n", err)
	}
}
