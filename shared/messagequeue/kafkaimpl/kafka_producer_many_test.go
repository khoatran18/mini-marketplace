package kafkaimpl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

// fakeWriter answers each call from a script; calls records the keys of every call.
type fakeWriter struct {
	script []error
	calls  [][]string
	heads  [][]kafka.Header
}

func (f *fakeWriter) WriteMessages(_ context.Context, msgs ...kafka.Message) error {
	keys := make([]string, len(msgs))
	for i, m := range msgs {
		keys[i] = string(m.Key)
		f.heads = append(f.heads, m.Headers)
	}
	f.calls = append(f.calls, keys)
	var err error
	if len(f.script) > 0 {
		err, f.script = f.script[0], f.script[1:]
	}
	return err
}

func msgsOf(keys ...string) []Message {
	out := make([]Message, len(keys))
	for i, k := range keys {
		out[i] = Message{Topic: "t", Key: k, Payload: []byte(`{}`)}
	}
	return out
}

func TestWriteGroupOneCallForAllMessagesAndRequestIDHeader(t *testing.T) {
	id := "req-1"
	m := msgsOf("a", "b", "c")
	m[1].RequestID = &id
	w := &fakeWriter{}
	errs := make([]error, 3)
	writeGroup(context.Background(), w, m, []int{0, 1, 2}, errs, 3, time.Millisecond)
	if len(w.calls) != 1 || len(w.calls[0]) != 3 || errs[0] != nil || errs[1] != nil || errs[2] != nil {
		t.Fatalf("calls=%v errs=%v", w.calls, errs)
	}
	if len(w.heads[1]) != 1 || string(w.heads[1][0].Value) != "req-1" || len(w.heads[0]) != 0 {
		t.Fatalf("headers=%v", w.heads)
	}
}

func TestWriteGroupRetriesOnlyTheMessagesThatFailed(t *testing.T) {
	w := &fakeWriter{script: []error{kafka.WriteErrors{nil, errors.New("leader moved"), nil}, nil}}
	errs := make([]error, 3)
	writeGroup(context.Background(), w, msgsOf("a", "b", "c"), []int{0, 1, 2}, errs, 3, time.Millisecond)
	if len(w.calls) != 2 || len(w.calls[1]) != 1 || w.calls[1][0] != "b" {
		t.Fatalf("calls=%v", w.calls)
	}
	for i, e := range errs {
		if e != nil {
			t.Fatalf("errs[%d]=%v", i, e)
		}
	}
}

func TestWriteGroupReportsEveryMessageWhenTheWholeCallKeepsFailing(t *testing.T) {
	boom := errors.New("broker down")
	w := &fakeWriter{script: []error{boom, boom}}
	errs := make([]error, 2)
	writeGroup(context.Background(), w, msgsOf("a", "b"), []int{0, 1}, errs, 2, time.Millisecond)
	if len(w.calls) != 2 || !errors.Is(errs[0], boom) || !errors.Is(errs[1], boom) {
		t.Fatalf("calls=%v errs=%v", w.calls, errs)
	}
}

func TestWriteGroupStopsRetryingWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &fakeWriter{script: []error{errors.New("x"), nil}}
	errs := make([]error, 1)
	writeGroup(ctx, w, msgsOf("a"), []int{0}, errs, 5, time.Hour)
	if errs[0] == nil || len(w.calls) != 1 {
		t.Fatalf("calls=%v errs=%v", w.calls, errs)
	}
}
