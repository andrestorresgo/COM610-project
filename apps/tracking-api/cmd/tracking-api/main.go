package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"tracking-api/internal/broker"
	"tracking-api/internal/cache"
)

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

	// 1. Connect to Redis with retry/backoff
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	var redisStore *cache.RedisStore
	var err error
	maxRetries := 5
	backoff := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempting to connect to Redis... (Attempt %d/%d)", i+1, maxRetries)
		redisStore, err = cache.NewRedisStore(redisURL)
		if err == nil {
			log.Println("Successfully connected to Redis!")
			break
		}
		log.Printf("Failed to connect: %v. Retrying in %v...", err, backoff)
		time.Sleep(backoff)
		backoff *= 2
	}

	if err != nil {
		log.Fatalf("Could not connect to Redis after %d attempts: %v", maxRetries, err)
	}

	// 1.5. Connect to RabbitMQ
	rabbitBroker, err := broker.Connect("", 5)
	if err != nil {
		log.Printf("Warning: Failed to connect to RabbitMQ: %v", err)
	} else {
		defer rabbitBroker.Close()
		if err := rabbitBroker.ConsumePingEvents(); err != nil {
			log.Printf("Warning: Failed to start consuming ping events: %v", err)
		} else {
			log.Println("Successfully started RabbitMQ consumer for ping events")
		}
	}

	// 2 & 3. Hardcode a dummy coordinate and call UpdateCourierLocation
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	lon, lat := -65.2592, -19.0333 // Coordinates for central Sucre
	courierID := "dummy-courier-001"

	log.Printf("Updating location for courier %s to (%f, %f)", courierID, lon, lat)
	err = redisStore.UpdateCourierLocation(ctx, courierID, lon, lat)
	if err != nil {
		log.Printf("Warning: Failed to update courier location: %v", err)
	} else {
		log.Println("Successfully updated courier location!")
	}

	// 4. Call GetNearbyCouriers and log the result
	searchCtx, searchCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer searchCancel()

	couriers, err := redisStore.GetNearbyCouriers(searchCtx, lon, lat, 5.0) // 5km radius
	if err != nil {
		log.Printf("Warning: Failed to search for couriers: %v", err)
	} else {
		log.Printf("Found %d nearby couriers:", len(couriers))
		for _, c := range couriers {
			log.Printf("- Courier: %s (Distance: %f)", c.Name, c.Dist)
		}
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/", rootHandler)

	log.Printf("Tracking API starting on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Could not start server: %v\n", err)
	}
}
