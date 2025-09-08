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

func (p *KafkaProducer) Publish(ctx context.Context, balance kafka.Balancer, topic string, key, value []byte) error {
	var writer *kafka.Writer
	if w, ok := p.km.writers[topic]; ok {
		writer = w
	} else {
		writer = p.km.NewWriter(topic, balance)
	}

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
