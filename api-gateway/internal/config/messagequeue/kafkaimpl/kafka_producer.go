package kafkaimpl

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaProducer struct {
	km      *KafkaManager
	retry   int
	backoff time.Duration
}

func NewKafkaProducer(km *KafkaManager, retry int, backoff time.Duration) *KafkaProducer {
	return &KafkaProducer{
		km:      km,
		retry:   retry,
		backoff: backoff,
	}
}

func (p *KafkaProducer) Publish(ctx context.Context, balance kafka.Balancer, topic string, key, value []byte) error {
	// Check if writer is existed
	var writer *kafka.Writer
	if w, ok := p.km.writers[topic]; ok {
		writer = w
		log.Println("writer is created")
	} else {
		writer = p.km.newWriter(topic, balance)
		log.Println("writer is nil and create")
	}

	// Get only first error, but need to publish all events
	var lastErr error
	for i := 0; i < p.retry; i++ {
		if err := writer.WriteMessages(ctx, kafka.Message{
			Key:   key,
			Value: value,
		}); err != nil {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.backoff):
				continue
			}
		}
		return nil
	}
	return lastErr
}
