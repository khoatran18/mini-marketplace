package kafkaimpl

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/segmentio/kafka-go"
)

// maxHandlerAttempts bounds how often a failing message is retried before it is
// moved to the dead-letter topic, so one poison message cannot block a partition forever.
const maxHandlerAttempts = 5

// DLQSuffix is appended to the source topic name to get its dead-letter topic.
const DLQSuffix = ".dlq"

var dlqTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "mm_kafka_dlq_total",
	Help: "Messages moved to the dead-letter topic (<topic>.dlq) after the handler retries were exhausted.",
}, []string{"topic"})

type KafkaConsumer struct {
	km      *KafkaManager
	backoff time.Duration

	// Overridable in tests; production values are set by NewKafkaConsumer.
	publishDLQ func(ctx context.Context, dlqTopic string, m kafka.Message) error
	ensureDLQ  func(ctx context.Context, dlqTopic string) error
}

func NewKafkaConsumer(km *KafkaManager, backoff time.Duration) *KafkaConsumer {
	c := &KafkaConsumer{
		km:      km,
		backoff: backoff,
	}
	c.publishDLQ = func(ctx context.Context, dlqTopic string, m kafka.Message) error {
		return km.newWriter(dlqTopic, &kafka.Hash{}).WriteMessages(ctx, m)
	}
	c.ensureDLQ = func(ctx context.Context, dlqTopic string) error {
		return NewKafkaClient(km.brokers).EnsureTopicExist(ctx, dlqTopic)
	}
	return c
}

type MessageHandler func(ctx context.Context, message *kafka.Message) error

// messageReader is the subset of *kafka.Reader used by the consume loop.
type messageReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
}

// Consume reads messages from topic and calls handler for each one.
// A message is committed only after the handler succeeds. On failure the handler is
// retried with a growing backoff; after maxHandlerAttempts the original message is
// published to the dead-letter topic "<topic>.dlq" (with error/attempt/source headers)
// and only then committed. If the DLQ publish fails it is retried until it succeeds (or
// ctx ends) and the message is NOT committed, so nothing is lost. Handlers must be
// idempotent (messages are delivered at least once).
func (c *KafkaConsumer) Consume(ctx context.Context, topic, groupID string, handler MessageHandler) error {
	return c.consumeLoop(ctx, c.km.NewReader(topic, groupID), topic, handler)
}

func (c *KafkaConsumer) consumeLoop(ctx context.Context, reader messageReader, topic string, handler MessageHandler) error {
	dlqTopic := topic + DLQSuffix
	if c.ensureDLQ != nil {
		if err := c.ensureDLQ(ctx, dlqTopic); err != nil {
			// Not fatal: the DLQ publish below retries and the topic may be auto-created or exist already.
			log.Printf("Consumer could not ensure DLQ topic %s: %v", dlqTopic, err)
		}
	}

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
			if derr := c.deadLetter(ctx, topic, dlqTopic, msg, err); derr != nil {
				return derr // ctx ended while the DLQ was unavailable: leave uncommitted
			}
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("Consumer committing messages failed topic: %s, error: %v", topic, err)
		}
	}
}

// deadLetter publishes msg to dlqTopic, retrying until it succeeds. It returns an error only
// when ctx ends first, in which case the caller must not commit.
func (c *KafkaConsumer) deadLetter(ctx context.Context, topic, dlqTopic string, msg kafka.Message, cause error) error {
	out := buildDLQMessage(topic, msg, cause, maxHandlerAttempts, time.Now())
	for try := 1; ; try++ {
		err := c.publishDLQ(ctx, dlqTopic, out)
		if err == nil {
			dlqTotal.WithLabelValues(topic).Inc()
			log.Printf("Consumer moved message to DLQ %s, partition: %d, offset: %d", dlqTopic, msg.Partition, msg.Offset)
			return nil
		}
		log.Printf("Consumer DLQ publish failed topic: %s, try %d, error: %v (message left uncommitted)", dlqTopic, try, err)
		if c.ensureDLQ != nil {
			_ = c.ensureDLQ(ctx, dlqTopic)
		}
		wait := c.backoff * time.Duration(try)
		if wait > 30*time.Second {
			wait = 30 * time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// buildDLQMessage keeps the original key, value and headers and appends dlq.* headers
// describing the failure and where the message came from.
func buildDLQMessage(topic string, msg kafka.Message, cause error, attempts int, now time.Time) kafka.Message {
	headers := make([]kafka.Header, 0, len(msg.Headers)+6)
	headers = append(headers, msg.Headers...)
	errText := ""
	if cause != nil {
		errText = cause.Error()
	}
	headers = append(headers,
		kafka.Header{Key: "dlq.error", Value: []byte(errText)},
		kafka.Header{Key: "dlq.attempts", Value: []byte(strconv.Itoa(attempts))},
		kafka.Header{Key: "dlq.source_topic", Value: []byte(topic)},
		kafka.Header{Key: "dlq.source_partition", Value: []byte(strconv.Itoa(msg.Partition))},
		kafka.Header{Key: "dlq.source_offset", Value: []byte(strconv.FormatInt(msg.Offset, 10))},
		kafka.Header{Key: "dlq.failed_at", Value: []byte(now.UTC().Format(time.RFC3339Nano))},
	)
	return kafka.Message{Key: msg.Key, Value: msg.Value, Headers: headers}
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
