package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "github.com/0xzaki/calc-service/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	amqp "github.com/rabbitmq/amqp091-go"

)

type server struct {
	pb.UnimplementedCalculatorServer
	rabbitMQConn *amqp.Connection
	rabbitMQChan *amqp.Channel
}

const (
	queueName = "sum_queue"
)

// publish to rabbitmq
func (s *server) publishToRabbitMQ(sum int32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body := fmt.Sprintf("%d", sum)
	err := s.rabbitMQChan.PublishWithContext(
		ctx,
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(body),
		},
	)
	if err != nil {
		return err
	}

	log.Printf("published sum %d to rabbitmq", sum)
	return nil
}

// sum function
func (s *server) Sum(ctx context.Context, req *pb.SumRequest) (*pb.SumResponse, error) {
	a := req.GetA()
	b := req.GetB()
	result := a + b

	log.Printf("computing sum: %d + %d = %d", a, b, result)

	// publish to rabbitmq
	err := s.publishToRabbitMQ(result)
	if err != nil {
		log.Printf("failed to publish to rabbitmq: %v", err)
	}
	return &pb.SumResponse{Result: result}, nil
}

// connect to rabbitmq 
func connectToRabbitMQ() (*amqp.Connection, *amqp.Channel, error) {
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return nil, nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}
	return conn, ch, nil
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	listen, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// connect to rabbitmq
	conn, ch, err := connectToRabbitMQ()
	if err != nil {
		log.Fatalf("failed xd to connect to rabbitmq: %v", err)
	} else {
		log.Printf("connected to rabbitmq")
		defer conn.Close()
		defer ch.Close()
	}


	// create grpc server
	s := grpc.NewServer()
	pb.RegisterCalculatorServer(s, &server{
		rabbitMQConn: conn,
		rabbitMQChan: ch,
	})

	// register reflection to use grpc cli
	reflection.Register(s)


	log.Printf("gRPC server started on port %s", port)

	if err := s.Serve(listen); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}