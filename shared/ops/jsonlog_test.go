package ops

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestJSONLogWriter(t *testing.T) {
	var buf bytes.Buffer
	w := &jsonLogWriter{out: &buf, service: "svc", now: func() time.Time { return time.Date(2026, 11, 12, 1, 2, 3, 0, time.UTC) }}
	if _, err := w.Write([]byte("Consumer handler error topic: x\n")); err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %q (%v)", buf.String(), err)
	}
	if m["service"] != "svc" || m["level"] != "warn" || m["msg"] != "Consumer handler error topic: x" || m["timestamp"] == "" {
		t.Fatalf("%v", m)
	}
	buf.Reset()
	_, _ = w.Write([]byte("listening at :50055"))
	_ = json.Unmarshal(buf.Bytes(), &m)
	if m["level"] != "info" {
		t.Fatalf("level = %s", m["level"])
	}
}
