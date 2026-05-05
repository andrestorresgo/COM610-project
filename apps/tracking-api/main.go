package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
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

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Hello from Tracking API!",
	})
}

func main() {
	healthcheck := flag.Bool("healthcheck", false, "run healthcheck")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if *healthcheck {
		resp, err := http.Get("http://localhost:" + port + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	http.HandleFunc("/health", healthHandler)

	http.HandleFunc("/", rootHandler)

	log.Printf("Tracking API starting on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Could not start server: %v\n", err)
	}
}
