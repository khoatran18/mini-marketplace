package kafkaimpl

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// maxHandlerAttempts bounds how often a failing message is retried before it is
// skipped, so one poison message cannot block a partition forever.
const maxHandlerAttempts = 5

type KafkaConsumer struct {
	km      *KafkaManager
	backoff time.Duration
}

func NewKafkaConsumer(km *KafkaManager, backoff time.Duration) *KafkaConsumer {
	return &KafkaConsumer{
		km:      km,
		backoff: backoff,
	}
}

type MessageHandler func(ctx context.Context, message *kafka.Message) error

// Consume reads messages from topic and calls handler for each one.
// A message is committed only after the handler succeeds. On failure the handler is
// retried with a growing backoff; after maxHandlerAttempts the message is logged and
// skipped. Handlers must therefore be idempotent (messages are delivered at least once).
func (c *KafkaConsumer) Consume(ctx context.Context, topic, groupID string, handler MessageHandler) error {
	reader := c.km.NewReader(topic, groupID)

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Printf("Consumer stopped for topic: %s", topic)
				return ctx.Err()
			}
			log.Printf("Consumer error reading for topic: %s, error: %v", topic, err)
			time.Sleep(c.backoff)
			continue
		}

		if err := c.handleWithRetry(ctx, topic, handler, &msg); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("Consumer giving up on message topic: %s, partition: %d, offset: %d, error: %v",
				topic, msg.Partition, msg.Offset, err)
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("Consumer committing messages failed topic: %s, error: %v", topic, err)
		}
	}
}

func (c *KafkaConsumer) handleWithRetry(ctx context.Context, topic string, handler MessageHandler, msg *kafka.Message) error {
	var err error
	for attempt := 1; attempt <= maxHandlerAttempts; attempt++ {
		if err = handler(ctx, msg); err == nil {
			return nil
		}
		log.Printf("Consumer handler error topic: %s, attempt %d/%d, error: %v", topic, attempt, maxHandlerAttempts, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.backoff * time.Duration(attempt)):
		}
	}
	return err
}
