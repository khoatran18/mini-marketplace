package kafkaimpl

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

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

func (c *KafkaConsumer) Consume(ctx context.Context, topic, groupID string, handler MessageHandler) error {
	var reader *kafka.Reader
	if r, ok := c.km.readers[topic]; ok {
		reader = r
	} else {
		reader = c.km.NewReader(topic, groupID)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("Consumer stopped for Topic: %s", topic)
			return ctx.Err()
		default:
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Printf("Consumer error reading for Topic: %s, Error: %v", topic, err)
				time.Sleep(c.backoff)
				continue
			}

			if err := handler(ctx, &msg); err != nil {
				log.Printf("Consumer handler error topic: %s, error: %v", topic, err)
				continue
			}
			if err := reader.CommitMessages(ctx, msg); err != nil {
				log.Printf("Consumer write to Redis successfully but commit messages topic failed: %s, error: %v", topic, err)
				continue
			}
			log.Printf("Consumer write to Redis and commit messages topic successfully: %s", topic)
		}
	}

}
