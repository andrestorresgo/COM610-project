package rabbitmq

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"tracking-api/internal/ws"
)

type Consumer struct {
	connURL    string
	hub        *ws.Hub
	maxRetries int
	done       chan struct{}
}

func NewConsumer(connURL string, hub *ws.Hub) *Consumer {
	if connURL == "" {
		connURL = os.Getenv("RABBITMQ_URL")
		if connURL == "" {
			connURL = "amqp://guest:guest@rabbitmq:5672/"
		}
	}
	return &Consumer{
		connURL:    connURL,
		hub:        hub,
		maxRetries: 10,
		done:       make(chan struct{}),
	}
}

// Start launches the consumer reconnection loop in a background goroutine
func (c *Consumer) Start() {
	go c.run()
}

// Close stops the consumer
func (c *Consumer) Close() {
	close(c.done)
}

func (c *Consumer) run() {
	for {
		select {
		case <-c.done:
			return
		default:
			err := c.connectAndConsume()
			if err != nil {
				log.Printf("[RabbitMQ Consumer] Connection lost or failed: %v", err)
			}

			select {
			case <-c.done:
				return
			default:
				log.Println("[RabbitMQ Consumer] Reconnecting in 5 seconds...")
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (c *Consumer) connectAndConsume() error {
	var conn *amqp.Connection
	var err error

	// Connect with exponential backoff retry logic
	for i := 0; i < c.maxRetries; i++ {
		select {
		case <-c.done:
			return nil
		default:
			log.Printf("[RabbitMQ Consumer] Attempting to connect (Attempt %d/%d)...", i+1, c.maxRetries)
			conn, err = amqp.Dial(c.connURL)
			if err == nil {
				break
			}
			waitTime := time.Duration(math.Pow(2, float64(i))) * time.Second
			if waitTime > 30*time.Second {
				waitTime = 30 * time.Second
			}
			log.Printf("[RabbitMQ Consumer] Failed to connect: %v. Retrying in %v...", err, waitTime)
			time.Sleep(waitTime)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to connect after %d retries: %w", c.maxRetries, err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}
	defer ch.Close()

	// 1. Declare the topic exchange (agachadeats_events)
	exchangeName := "agachadeats_events"
	err = ch.ExchangeDeclare(
		exchangeName,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	// 2. Declare the Dead Letter Exchange (DLX)
	dlxName := "agachadeats_events.dlx"
	err = ch.ExchangeDeclare(
		dlxName,
		"fanout",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare dead letter exchange: %w", err)
	}

	// 3. Declare the Dead Letter Queue (DLQ)
	dlqName := "delivery_orders_queue_dlq"
	_, err = ch.QueueDeclare(
		dlqName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare DLQ: %w", err)
	}

	// 4. Bind the DLQ to the DLX
	err = ch.QueueBind(
		dlqName,
		"",
		dlxName,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind DLQ to DLX: %w", err)
	}

	// 5. Declare the primary consumer queue (delivery_orders_queue) with DLX arguments
	mainQueueName := "delivery_orders_queue"
	args := amqp.Table{
		"x-dead-letter-exchange": dlxName,
	}
	_, err = ch.QueueDeclare(
		mainQueueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		args,  // arguments containing DLX configuration
	)
	if err != nil {
		return fmt.Errorf("failed to declare main queue: %w", err)
	}

	// 6. Bind the main queue to the main exchange with 'order.ready' routing key
	routingKey := "order.ready"
	err = ch.QueueBind(
		mainQueueName,
		routingKey,
		exchangeName,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind main queue: %w", err)
	}

	// 7. Register consumer
	msgs, err := ch.Consume(
		mainQueueName,
		"tracking-api-consumer",
		false, // manual ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	// Setup channels to watch for connection/channel closures
	connClosed := conn.NotifyClose(make(chan *amqp.Error))
	chanClosed := ch.NotifyClose(make(chan *amqp.Error))

	log.Println("[RabbitMQ Consumer] Connected and listening for order.ready events")

	for {
		select {
		case <-c.done:
			return nil
		case errErr := <-connClosed:
			return fmt.Errorf("connection closed by server: %v", errErr)
		case errErr := <-chanClosed:
			return fmt.Errorf("channel closed by server: %v", errErr)
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			// Process event
			var event ws.OrderReadyEvent
			err := json.Unmarshal(msg.Body, &event)
			if err != nil {
				log.Printf("[RabbitMQ Consumer] Failed to unmarshal message. Routing to DLQ. Error: %v", err)
				// Nack with requeue = false, which pushes the message to DLX/DLQ
				_ = msg.Nack(false, false)
				continue
			}

			// Validate payload contract
			if event.OrderID == 0 || event.ClientID == 0 || event.CourierID == 0 {
				log.Printf("[RabbitMQ Consumer] Invalid event payload contract. Routing to DLQ. Payload: %s", string(msg.Body))
				_ = msg.Nack(false, false)
				continue
			}

			// Send to WebSocket Hub
			select {
			case c.hub.OrderReady <- event:
				log.Printf("[RabbitMQ Consumer] Handed off OrderReady event to Hub: Order=%d, Client=%d, Courier=%d", event.OrderID, event.ClientID, event.CourierID)
				_ = msg.Ack(false)
			case <-c.done:
				// If shutting down, reject and requeue
				_ = msg.Nack(false, true)
				return nil
			}
		}
	}
}
