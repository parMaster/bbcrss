package main

import (
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Mq struct {
	name string
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewMq creates new connection to RabbitMQ
func NewMq(cfg RMQConfig) (*Mq, error) {
	conn, err := amqp.Dial(cfg.Dsn)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to connect to RabbitMQ %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to open a channel %w", err)
	}

	// declare a queue for news items with default settings
	_, err = ch.QueueDeclare(
		cfg.Name, // name
		false,    // durable
		false,    // delete when unused
		false,    // exclusive
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to declare a queue %w", err)
	}

	return &Mq{name: cfg.Name, conn: conn, ch: ch}, nil
}

// Close closes connection to RabbitMQ
func (mq *Mq) Close() error {
	var err error

	err = mq.ch.Close()
	if err != nil {
		log.Printf("failed to close RabbitMQ channel %v", err)
	}

	err = mq.conn.Close()
	if err != nil {
		log.Printf("failed to close RabbitMQ connection %v", err)
	}

	return err
}

// Publish sends message to RabbitMQ
func (mq *Mq) Publish(msg []byte) error {
	err := mq.ch.Publish(
		"",      // exchange
		mq.name, // routing key
		false,   // mandatory
		false,   // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        msg,
		})
	if err != nil {
		return fmt.Errorf("[ERROR] failed to publish a message %w", err)
	}

	return nil
}

// Consume returns channel with messages from RabbitMQ
func (mq *Mq) Consume() (<-chan amqp.Delivery, error) {
	msgs, err := mq.ch.Consume(
		mq.name, // queue
		"",      // consumer
		true,    // auto-ack
		false,   // exclusive
		false,   // no-local
		false,   // no-wait
		nil,     // args
	)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to consume messages %w", err)
	}

	return msgs, nil
}
