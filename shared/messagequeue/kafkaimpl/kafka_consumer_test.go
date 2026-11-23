package kafkaimpl

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/segmentio/kafka-go"
)

type fakeReader struct {
	msgs      []kafka.Message
	committed []kafka.Message
	cancel    context.CancelFunc
}

func (f *fakeReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	if len(f.msgs) == 0 {
		f.cancel()
		return kafka.Message{}, ctx.Err()
	}
	m := f.msgs[0]
	f.msgs = f.msgs[1:]
	return m, nil
}

func (f *fakeReader) CommitMessages(_ context.Context, m ...kafka.Message) error {
	f.committed = append(f.committed, m...)
	return nil
}

func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	return m.GetCounter().GetValue()
}

func newTestConsumer() *KafkaConsumer {
	return &KafkaConsumer{backoff: time.Millisecond}
}

func TestBuildDLQMessage(t *testing.T) {
	in := kafka.Message{Key: []byte("k"), Value: []byte("v"), Partition: 2, Offset: 41,
		Headers: []kafka.Header{{Key: "x", Value: []byte("1")}}}
	out := buildDLQMessage("orders", in, errors.New("boom"), 5, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if string(out.Key) != "k" || string(out.Value) != "v" {
		t.Fatalf("key/value not preserved: %+v", out)
	}
	h := map[string]string{}
	for _, x := range out.Headers {
		h[x.Key] = string(x.Value)
	}
	want := map[string]string{"x": "1", "dlq.error": "boom", "dlq.attempts": "5", "dlq.source_topic": "orders",
		"dlq.source_partition": "2", "dlq.source_offset": "41", "dlq.failed_at": "2026-01-02T03:04:05Z"}
	for k, v := range want {
		if h[k] != v {
			t.Errorf("header %s = %q, want %q", k, h[k], v)
		}
	}
}

func TestConsumeSuccessDoesNotDeadLetter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &fakeReader{msgs: []kafka.Message{{Offset: 1}}, cancel: cancel}
	c := newTestConsumer()
	c.publishDLQ = func(context.Context, string, kafka.Message) error { t.Fatal("unexpected DLQ publish"); return nil }
	_ = c.consumeLoop(ctx, r, "t", func(context.Context, *kafka.Message) error { return nil })
	if len(r.committed) != 1 {
		t.Fatalf("committed %d, want 1", len(r.committed))
	}
}

func TestConsumeExhaustedRetriesGoToDLQThenCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &fakeReader{msgs: []kafka.Message{{Key: []byte("k"), Value: []byte("v"), Offset: 7}}, cancel: cancel}
	c := newTestConsumer()
	var gotTopic string
	var got kafka.Message
	var published int
	var committedAtPublish int
	c.publishDLQ = func(_ context.Context, topic string, m kafka.Message) error {
		published++
		committedAtPublish = len(r.committed)
		gotTopic, got = topic, m
		return nil
	}
	attempts := 0
	before := counterValue(dlqTotal.WithLabelValues("t"))
	_ = c.consumeLoop(ctx, r, "t", func(context.Context, *kafka.Message) error { attempts++; return errors.New("bad") })
	if attempts != maxHandlerAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, maxHandlerAttempts)
	}
	if published != 1 || gotTopic != "t.dlq" || string(got.Value) != "v" {
		t.Fatalf("dlq publish wrong: n=%d topic=%s msg=%+v", published, gotTopic, got)
	}
	if committedAtPublish != 0 || len(r.committed) != 1 {
		t.Fatalf("commit must happen after publish: atPublish=%d total=%d", committedAtPublish, len(r.committed))
	}
	if d := counterValue(dlqTotal.WithLabelValues("t")) - before; d != 1 {
		t.Fatalf("metric delta = %v, want 1", d)
	}
}

func TestConsumeDLQFailureRetriesAndDoesNotCommitUntilPublished(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &fakeReader{msgs: []kafka.Message{{Offset: 1}}, cancel: cancel}
	c := newTestConsumer()
	calls := 0
	c.publishDLQ = func(context.Context, string, kafka.Message) error {
		calls++
		if calls < 3 {
			return errors.New("kafka down")
		}
		if len(r.committed) != 0 {
			t.Error("committed before DLQ publish succeeded")
		}
		return nil
	}
	_ = c.consumeLoop(ctx, r, "t", func(context.Context, *kafka.Message) error { return errors.New("bad") })
	if calls != 3 || len(r.committed) != 1 {
		t.Fatalf("calls=%d committed=%d", calls, len(r.committed))
	}
}

func TestConsumeDLQDownAndShutdownLeavesUncommitted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &fakeReader{msgs: []kafka.Message{{Offset: 1}}, cancel: cancel}
	c := newTestConsumer()
	c.publishDLQ = func(context.Context, string, kafka.Message) error { cancel(); return errors.New("down") }
	err := c.consumeLoop(ctx, r, "t", func(context.Context, *kafka.Message) error { return errors.New("bad") })
	if err == nil {
		t.Fatal("expected ctx error")
	}
	if len(r.committed) != 0 {
		t.Fatal("message must not be committed when DLQ publish failed")
	}
}

func TestWorkersFromEnv(t *testing.T) {
	for in, want := range map[string]int{"": 1, "x": 1, "0": 1, "-2": 1, "3": 3, "64": 64, "1000": 64} {
		t.Setenv("KAFKA_CONSUMER_WORKERS", in)
		if got := workersFromEnv(); got != want {
			t.Errorf("KAFKA_CONSUMER_WORKERS=%q: got %d, want %d", in, got, want)
		}
	}
}

// With 3 workers the handlers of 3 readers run at the same time (a sequential loop would never get all 3 inside the
// barrier) and every message is committed by the reader that fetched it.
func TestConsumeWorkersRunInParallel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readers := []*fakeReader{
		{msgs: []kafka.Message{{Partition: 0, Offset: 1}}, cancel: cancel},
		{msgs: []kafka.Message{{Partition: 1, Offset: 1}}, cancel: cancel},
		{msgs: []kafka.Message{{Partition: 2, Offset: 1}}, cancel: cancel},
	}
	next := 0
	newReader := func() messageReader { r := readers[next]; next++; return r }
	var inside sync.WaitGroup
	inside.Add(3)
	allIn := make(chan struct{})
	go func() { inside.Wait(); close(allIn) }()
	handler := func(context.Context, *kafka.Message) error {
		inside.Done()
		select {
		case <-allIn:
			return nil
		case <-time.After(2 * time.Second):
			return errors.New("handlers did not overlap")
		}
	}
	c := newTestConsumer()
	c.publishDLQ = func(context.Context, string, kafka.Message) error { t.Error("unexpected DLQ publish"); return nil }
	_ = c.consumeWorkers(ctx, 3, newReader, "t", handler)
	for i, r := range readers {
		if len(r.committed) != 1 {
			t.Errorf("reader %d committed %d, want 1", i, len(r.committed))
		}
	}
}

func TestConsumeWorkersOneIsTheSequentialLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &fakeReader{msgs: []kafka.Message{{Offset: 1}, {Offset: 2}}, cancel: cancel}
	built := 0
	_ = newTestConsumer().consumeWorkers(ctx, 1, func() messageReader { built++; return r }, "t", func(context.Context, *kafka.Message) error { return nil })
	if built != 1 || len(r.committed) != 2 {
		t.Fatalf("built=%d committed=%d, want 1 and 2", built, len(r.committed))
	}
}

func TestCommitIntervalFromEnv(t *testing.T) {
	for in, want := range map[string]time.Duration{"": 0, "x": 0, "-5": 0, "0": 0, "100": 100 * time.Millisecond, "999999": time.Minute} {
		t.Setenv("KAFKA_COMMIT_INTERVAL_MS", in)
		if got := commitIntervalFromEnv(); got != want {
			t.Errorf("KAFKA_COMMIT_INTERVAL_MS=%q: got %v, want %v", in, got, want)
		}
	}
	t.Setenv("KAFKA_COMMIT_INTERVAL_MS", "100")
	if NewKafkaManager(nil).commitInterval != 100*time.Millisecond {
		t.Error("manager must pick up the commit interval")
	}
}

// Workers must never share a reader: two goroutines on one reader could process consecutive messages of a partition
// at once. Consume builds one reader per worker (not the cached one).
func TestConsumeWorkersUseDistinctReaders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var made []messageReader
	newReader := func() messageReader { r := &fakeReader{cancel: cancel}; made = append(made, r); return r }
	_ = newTestConsumer().consumeWorkers(ctx, 3, newReader, "t", func(context.Context, *kafka.Message) error { return nil })
	if len(made) != 3 || made[0] == made[1] || made[1] == made[2] {
		t.Fatalf("want 3 distinct readers, got %d", len(made))
	}
}
