package queue

import (
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

var Conn *amqp.Connection

func Init(url string) {
	var err error
	Conn, err = amqp.Dial(url)
	if err != nil {
		log.Fatalf("❌ Failed to connect to RabbitMQ: %v", err)
	}
	log.Println("✅ Connected to RabbitMQ")
}
