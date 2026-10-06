package kafkaimpl

import (
	"context"
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

// Publish writes one message. Messages with the same key go to the same partition
// when a hash balancer is used, which preserves per-key ordering.
func (p *KafkaProducer) Publish(ctx context.Context, balance kafka.Balancer, topic string, key, value []byte) error {
	writer := p.km.newWriter(topic, balance)

	attempts := p.retry
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		err := writer.WriteMessages(ctx, kafka.Message{
			Key:   key,
			Value: value,
		})
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.backoff):
		}
	}
	return lastErr
}
