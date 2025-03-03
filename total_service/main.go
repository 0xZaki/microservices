package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	queueName = "sum_queue"
	dataFile  = "data/total.txt"
)

type Consumer struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	totalFile string
	mutex     sync.Mutex
}

// create new rabbitmq consumer
func NewConsumer(amqpURL string, totalFile string) (*Consumer, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("error connecting to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("error creating channel: %w", err)
	}

	_, err = channel.QueueDeclare(
		queueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("error declaring queue: %w", err)
	}

	return &Consumer{
		conn:      conn,
		channel:   channel,
		totalFile: totalFile,
	}, nil
}

// Close closes the connection and channel
func (c *Consumer) Close() {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// read current total from the file
func (c *Consumer) ReadTotal() (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if file exists, create with 0 if it doesn't
	if _, err := os.Stat(c.totalFile); os.IsNotExist(err) {
		err = ioutil.WriteFile(c.totalFile, []byte("0"), 0644)
		if err != nil {
			return 0, fmt.Errorf("error creating total file: %w", err)
		}
		return 0, nil
	}

	data, err := ioutil.ReadFile(c.totalFile)
	if err != nil {
		return 0, fmt.Errorf("error reading total file: %w", err)
	}

	total, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, fmt.Errorf("error parsing total from file: %w", err)
	}

	return total, nil
}

// update total in the file
func (c *Consumer) UpdateTotal(newTotal int) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	err := ioutil.WriteFile(c.totalFile, []byte(strconv.Itoa(newTotal)), 0644)
	if err != nil {
		return fmt.Errorf("error writing total to file: %w", err)
	}

	return nil
}

// start consuming messages from the queue
func (c *Consumer) StartConsuming() error {
	msgs, err := c.channel.Consume(
		queueName,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("error consuming from queue: %w", err)
	}

	log.Printf("started consuming from queue: %s", queueName)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for msg := range msgs {
			sum, err := strconv.Atoi(string(msg.Body))
			if err != nil {
				log.Printf("Error parsing message body: %v", err)
				continue
			}

			log.Printf("Received sum: %d", sum)

			total, err := c.ReadTotal()
			if err != nil {
				log.Printf("Error reading total: %v", err)
				continue
			}

			newTotal := total + sum
			log.Printf("Updating total: %d + %d = %d", total, sum, newTotal)

			err = c.UpdateTotal(newTotal)
			if err != nil {
				log.Printf("Error updating total: %v", err)
				continue
			}

			log.Printf("Total updated successfully: %d", newTotal)
		}
	}()

	<-sigChan
	return nil
}

func main() {
	// create directory if it doesn't exist
	err := os.MkdirAll("data", 0755)
	if err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}


	amqpURL := os.Getenv("RABBITMQ_URL")
	if amqpURL == "" {
		amqpURL = "amqp://guest:guest@localhost:5672/"
	}

	// create rabbitmq consumer
	consumer, err := NewConsumer(amqpURL, dataFile)
	if err != nil {
		log.Printf("Error connecting to RabbitMQ: %v", err)
		return
	}
	defer consumer.Close()

	// start consuming
	log.Printf("Connected to RabbitMQ at %s", amqpURL)
	if err := consumer.StartConsuming(); err != nil {
		log.Fatalf("Error consuming messages: %v", err)
	}
} 