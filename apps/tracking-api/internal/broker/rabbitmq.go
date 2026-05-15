package broker

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchangeName = "agachadeats_events"
)

type RabbitMQBroker struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

// Connect starts a connection with exponential backoff
func Connect(url string, maxRetries int) (*RabbitMQBroker, error) {
	if url == "" {
		url = os.Getenv("RABBITMQ_URL")
		if url == "" {
			url = "amqp://guest:guest@rabbitmq:5672/"
		}
	}

	var conn *amqp.Connection
	var err error

	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempting to connect to RabbitMQ (Attempt %d/%d)...", i+1, maxRetries)
		conn, err = amqp.Dial(url)
		if err == nil {
			log.Println("Successfully connected to RabbitMQ")
			break
		}

		log.Printf("Failed to connect: %v", err)
		if i < maxRetries-1 {
			waitTime := time.Duration(math.Pow(2, float64(i))) * time.Second
			log.Printf("Retrying in %v...", waitTime)
			time.Sleep(waitTime)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ after retries: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	err = ch.ExchangeDeclare(
		exchangeName, // name
		"topic",      // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare an exchange: %w", err)
	}

	return &RabbitMQBroker{
		conn:    conn,
		channel: ch,
	}, nil
}

func (b *RabbitMQBroker) Close() {
	if b.channel != nil {
		b.channel.Close()
	}
	if b.conn != nil {
		b.conn.Close()
	}
}

// ConsumePingEvents binds an exclusive queue to the exchange and consumes "system.#"
func (b *RabbitMQBroker) ConsumePingEvents() error {
	q, err := b.channel.QueueDeclare(
		"",    // name (empty means a random name will be generated)
		false, // durable
		false, // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare a queue: %w", err)
	}

	// Bind the queue to the exchange
	err = b.channel.QueueBind(
		q.Name,       // queue name
		"system.#",   // routing key
		exchangeName, // exchange
		false,        // no-wait
		nil,          // args
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue: %w", err)
	}

	msgs, err := b.channel.Consume(
		q.Name, // queue
		"",     // consumer tag
		false,  // auto-ack (we use manual ack)
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return fmt.Errorf("failed to register a consumer: %w", err)
	}

	// Idempotency check cache
	seenMsgs := make(map[int64]bool)

	log.Println("Waiting for events. To exit press CTRL+C")

	go func() {
		for d := range msgs {
			var payload struct {
				Msg       string `json:"msg"`
				Timestamp int64  `json:"timestamp"`
			}

			if err := json.Unmarshal(d.Body, &payload); err != nil {
				log.Printf("Error unmarshaling message body: %v", err)
				d.Nack(false, false)
				continue
			}

			if seenMsgs[payload.Timestamp] {
				log.Printf("Duplicate ping event detected: %d. Acknowledging and discarding.", payload.Timestamp)
				d.Ack(false)
				continue
			}

			log.Printf("Received event [RoutingKey: %s]: %s (Timestamp: %d)", d.RoutingKey, payload.Msg, payload.Timestamp)
			seenMsgs[payload.Timestamp] = true
			d.Ack(false)
		}
	}()

	return nil
}
