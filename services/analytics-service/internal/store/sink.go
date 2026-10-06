package store

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"analytics-service/internal/ch"
)

// Time formats a time the way ClickHouse parses DateTime64 in its default input mode (UTC).
func Time(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format("2006-01-02 15:04:05.000")
}

type pending struct {
	table string
	row   any
	at    time.Time
}

// Sink buffers rows in memory and inserts them in batches (every FlushInterval, or when FlushRows are waiting).
// A failed insert keeps its rows for the next round; when BufferMax is exceeded the oldest rows are dropped and
// counted (a ClickHouse outage must not take the service down). Rows are idempotent in ClickHouse (Replacing
// engines), so a retry that partly succeeded before is harmless.
type Sink struct {
	db            *ch.Client
	flushInterval time.Duration
	flushRows     int
	bufferMax     int

	mu    sync.Mutex
	buf   []pending
	wake  chan struct{}
	stats struct {
		inserted, failedBatches, dropped atomic.Int64
	}
	lastOK atomic.Int64 // unix seconds of the last successful insert
}

// NewSink creates a sink; call Run to start the flusher.
func NewSink(db *ch.Client, flushInterval time.Duration, flushRows, bufferMax int) *Sink {
	if flushInterval <= 0 {
		flushInterval = 2 * time.Second
	}
	if flushRows <= 0 {
		flushRows = 5000
	}
	if bufferMax < flushRows {
		bufferMax = flushRows * 10
	}
	s := &Sink{db: db, flushInterval: flushInterval, flushRows: flushRows, bufferMax: bufferMax, wake: make(chan struct{}, 1)}
	s.lastOK.Store(time.Now().Unix())
	return s
}

// Add queues one row (a struct/map with json tags matching the table columns).
func (s *Sink) Add(table string, row any) {
	s.mu.Lock()
	s.buf = append(s.buf, pending{table: table, row: row, at: time.Now()})
	if over := len(s.buf) - s.bufferMax; over > 0 {
		s.buf = s.buf[over:]
		s.stats.dropped.Add(int64(over))
	}
	full := len(s.buf) >= s.flushRows
	s.mu.Unlock()
	if full {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// Run flushes until ctx is done, then flushes once more.
func (s *Sink) Run(ctx context.Context) {
	t := time.NewTicker(s.flushInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = s.Flush(fctx)
			cancel()
			return
		case <-t.C:
		case <-s.wake:
		}
		_ = s.Flush(ctx)
	}
}

// Flush inserts everything buffered now. Rows of a failing table stay queued.
func (s *Sink) Flush(ctx context.Context) error {
	s.mu.Lock()
	batch := s.buf
	s.buf = nil
	s.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	byTable := map[string][]any{}
	for _, p := range batch {
		byTable[p.table] = append(byTable[p.table], p.row)
	}
	var firstErr error
	var requeue []pending
	for table, rows := range byTable {
		if err := s.db.Insert(ctx, table, rows); err != nil {
			s.stats.failedBatches.Add(1)
			if firstErr == nil {
				firstErr = err
			}
			for _, p := range batch {
				if p.table == table {
					requeue = append(requeue, p)
				}
			}
			continue
		}
		s.stats.inserted.Add(int64(len(rows)))
		s.lastOK.Store(time.Now().Unix())
	}
	if len(requeue) > 0 {
		s.mu.Lock()
		s.buf = append(requeue, s.buf...)
		if over := len(s.buf) - s.bufferMax; over > 0 {
			s.buf = s.buf[over:]
			s.stats.dropped.Add(int64(over))
		}
		s.mu.Unlock()
	}
	return firstErr
}

// Stats is a snapshot for metrics and the data-health report.
type SinkStats struct {
	Buffered         int
	OldestAgeSeconds float64
	Inserted         int64
	FailedBatches    int64
	Dropped          int64
	LastSuccess      time.Time
}

// Stats returns the current counters.
func (s *Sink) Stats() SinkStats {
	s.mu.Lock()
	n := len(s.buf)
	var age float64
	if n > 0 {
		age = time.Since(s.buf[0].at).Seconds()
	}
	s.mu.Unlock()
	return SinkStats{Buffered: n, OldestAgeSeconds: age, Inserted: s.stats.inserted.Load(),
		FailedBatches: s.stats.failedBatches.Load(), Dropped: s.stats.dropped.Load(), LastSuccess: time.Unix(s.lastOK.Load(), 0)}
}
