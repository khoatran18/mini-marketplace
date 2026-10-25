package store

import (
	"context"
	"encoding/json"
	"log"
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
	spool         *Spool // optional durable overflow/shutdown storage (nil = disabled)

	mu    sync.Mutex
	buf   []pending
	wake  chan struct{}
	stats struct {
		inserted, failedBatches, dropped, spooled, replayed atomic.Int64
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

// WithSpool enables the durable spool: rows that overflow the RAM buffer, and rows still buffered at shutdown, are
// written to it instead of being dropped/lost, and replayed into ClickHouse once inserts work again (and at startup).
func (s *Sink) WithSpool(sp *Spool) *Sink {
	s.spool = sp
	return s
}

// overflow handles rows evicted from the RAM buffer: spool them when possible, otherwise count them as dropped.
func (s *Sink) overflow(rows []pending) {
	if len(rows) == 0 {
		return
	}
	stored := 0
	if s.spool != nil {
		n, err := s.spool.Append(rows)
		if err != nil {
			log.Printf("analytics spool: append failed: %v", err)
		}
		stored = n
		s.stats.spooled.Add(int64(n))
	}
	if d := len(rows) - stored; d > 0 {
		s.stats.dropped.Add(int64(d))
	}
}

// Add queues one row (a struct/map with json tags matching the table columns).
func (s *Sink) Add(table string, row any) {
	s.mu.Lock()
	s.buf = append(s.buf, pending{table: table, row: row, at: time.Now()})
	var evicted []pending
	if over := len(s.buf) - s.bufferMax; over > 0 {
		evicted = append(evicted, s.buf[:over]...)
		s.buf = s.buf[over:]
	}
	full := len(s.buf) >= s.flushRows
	s.mu.Unlock()
	s.overflow(evicted)
	if full {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// Run replays the spool, then flushes until ctx is done; on shutdown it flushes once more and spools what is left.
func (s *Sink) Run(ctx context.Context) {
	s.Replay(ctx)
	t := time.NewTicker(s.flushInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = s.Flush(fctx)
			cancel()
			s.spoolRemaining()
			return
		case <-t.C:
		case <-s.wake:
		}
		if err := s.Flush(ctx); err == nil && s.spool != nil && s.spool.Size() > 0 {
			s.Replay(ctx)
		}
	}
}

// spoolRemaining moves whatever is still in RAM to the spool (graceful shutdown with ClickHouse down).
func (s *Sink) spoolRemaining() {
	s.mu.Lock()
	rest := s.buf
	s.buf = nil
	s.mu.Unlock()
	s.overflow(rest)
}

// Replay inserts the spooled rows into ClickHouse in batches per table. Rows that fail stay in the spool; the file is
// removed once everything is in. Safe to call repeatedly.
func (s *Sink) Replay(ctx context.Context) {
	if s.spool == nil {
		return
	}
	sp := s.spool
	sp.mu.Lock()
	defer sp.mu.Unlock()
	recs, corrupt, err := sp.readAll()
	if err != nil {
		log.Printf("analytics spool: read failed: %v", err)
		return
	}
	if corrupt > 0 {
		s.stats.dropped.Add(int64(corrupt))
	}
	if len(recs) == 0 && corrupt == 0 {
		return
	}
	byTable := map[string][]any{}
	for _, r := range recs {
		byTable[r.Table] = append(byTable[r.Table], json.RawMessage(r.Row))
	}
	var left []spoolRecord
	for table, rows := range byTable {
		if err := s.db.Insert(ctx, table, rows); err != nil {
			s.stats.failedBatches.Add(1)
			for _, r := range recs {
				if r.Table == table {
					left = append(left, r)
				}
			}
			continue
		}
		s.stats.replayed.Add(int64(len(rows)))
		s.lastOK.Store(time.Now().Unix())
	}
	if err := sp.rewrite(left); err != nil {
		log.Printf("analytics spool: rewrite failed: %v", err)
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
		var evicted []pending
		if over := len(s.buf) - s.bufferMax; over > 0 {
			evicted = append(evicted, s.buf[:over]...)
			s.buf = s.buf[over:]
		}
		s.mu.Unlock()
		s.overflow(evicted)
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
	Spooled          int64 // rows written to the durable spool
	Replayed         int64 // rows replayed from the spool into ClickHouse
	SpoolBytes       int64
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
	var sb int64
	if s.spool != nil {
		sb = s.spool.Size()
	}
	return SinkStats{Spooled: s.stats.spooled.Load(), Replayed: s.stats.replayed.Load(), SpoolBytes: sb, Buffered: n, OldestAgeSeconds: age, Inserted: s.stats.inserted.Load(),
		FailedBatches: s.stats.failedBatches.Load(), Dropped: s.stats.dropped.Load(), LastSuccess: time.Unix(s.lastOK.Load(), 0)}
}
