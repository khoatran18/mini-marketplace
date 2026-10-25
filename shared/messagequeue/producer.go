package messagequeue

import (
	"context"

	"github.com/segmentio/kafka-go"
)

type Producer interface {
	Publish(ctx context.Context, balance kafka.Balancer, topic string, key, value []byte) error
}
