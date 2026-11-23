package kafkaimpl

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"

	"mini-marketplace/shared/events"
	"mini-marketplace/shared/reqctx"
)

// messageWriter is the part of *kafka.Writer used here (a fake in tests).
type messageWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
}

// PublishMany writes msgs with ONE WriteMessages call per topic (topics in parallel) instead of one call per message, and
// returns one error per message in order (nil = acknowledged by all replicas). Used by the outbox (events.RunMany): a call per
// message costs a full round trip plus the writer's batch timeout each, which capped a service at a few dozen events/s.
// Messages of the same key in one call keep their order (hash balancer); the outbox sends at most one row per key per batch anyway.
// Failed messages are retried like Publish (p.retry attempts, p.backoff between them), only the failed ones each time.
func (p *KafkaProducer) PublishMany(ctx context.Context, balance kafka.Balancer, msgs []events.Message) []error {
	errs := make([]error, len(msgs))
	byTopic := map[string][]int{}
	for i, m := range msgs {
		byTopic[m.Topic] = append(byTopic[m.Topic], i)
	}
	var wg sync.WaitGroup
	for topic, idx := range byTopic {
		wg.Add(1)
		go func(topic string, idx []int) {
			defer wg.Done()
			writeGroup(ctx, p.km.newWriter(topic, balance), msgs, idx, errs, p.retry, p.backoff)
		}(topic, idx)
	}
	wg.Wait()
	return errs
}

// writeGroup sends msgs[idx...] (all of one topic) through w and fills errs for those indexes.
func writeGroup(ctx context.Context, w messageWriter, msgs []events.Message, idx []int, errs []error, retry int, backoff time.Duration) {
	attempts := retry
	if attempts < 1 {
		attempts = 1
	}
	pending := idx
	for attempt := 0; attempt < attempts && len(pending) > 0; attempt++ {
		kmsgs := make([]kafka.Message, len(pending))
		for j, i := range pending {
			km := kafka.Message{Key: []byte(msgs[i].Key), Value: msgs[i].Payload}
			if msgs[i].RequestID != nil && *msgs[i].RequestID != "" {
				km.Headers = []kafka.Header{{Key: reqctx.Header, Value: []byte(*msgs[i].RequestID)}}
			}
			kmsgs[j] = km
		}
		err := w.WriteMessages(ctx, kmsgs...)
		var next []int
		var we kafka.WriteErrors
		switch {
		case err == nil:
			for _, i := range pending {
				errs[i] = nil
			}
		case errors.As(err, &we) && len(we) == len(pending): // per-message results
			for j, i := range pending {
				errs[i] = we[j]
				if we[j] != nil {
					next = append(next, i)
				}
			}
		default: // the whole call failed
			for _, i := range pending {
				errs[i] = err
				next = append(next, i)
			}
		}
		pending = next
		if len(pending) > 0 && attempt+1 < attempts {
			select {
			case <-ctx.Done():
				for _, i := range pending {
					errs[i] = ctx.Err()
				}
				return
			case <-time.After(backoff):
			}
		}
	}
}
