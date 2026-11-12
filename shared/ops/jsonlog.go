package ops

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// jsonLogWriter turns each stdlib log line into one JSON object with the same keys as the zap services
// (timestamp, level, msg) plus "service", so Loki/Vector parse every service the same way (ADR-36).
type jsonLogWriter struct {
	mu      sync.Mutex
	out     io.Writer
	service string
	now     func() time.Time
}

func (w *jsonLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	level := "info"
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "panic"):
		level = "error"
	case strings.Contains(low, "error") || strings.Contains(low, "fatal") || strings.Contains(low, "failed"):
		level = "warn"
	}
	b, _ := json.Marshal(map[string]string{"timestamp": w.now().UTC().Format("2006-01-02T15:04:05.000Z0700"), "level": level, "service": w.service, "msg": msg})
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.out.Write(append(b, '\n')); err != nil {
		return 0, err
	}
	return len(p), nil
}

// JSONStdLog makes the standard `log` package write JSON lines to stderr. For services that do not use zap.
func JSONStdLog(service string) {
	log.SetFlags(0)
	log.SetOutput(&jsonLogWriter{out: os.Stderr, service: service, now: time.Now})
}
