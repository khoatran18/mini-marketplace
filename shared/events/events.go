// Package events is a tiny transactional outbox shared by the services (module mini-marketplace/shared, ADR-33).
// A business change calls Emit inside its own database transaction; a worker
// (Run / PublishBatch) later publishes the rows to Kafka. Delivery is at-least-once, so consumers must be
// idempotent (the key and payload carry natural ids for that).
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mini-marketplace/shared/messagequeue/kafkaimpl"
	"mini-marketplace/shared/reqctx"
)

const (
	StatusPending = "PENDING"
	StatusFailed  = "FAILED"
	StatusDone    = "SUCCESS"
	// StatusDead is the dead-letter state: the row failed Options.MaxAttempts times, is kept for inspection
	// and is NOT retried automatically (requeue by hand, see docs/instruction/03_RUNBOOK.md section 5).
	StatusDead = "DEAD"
)

// DefaultTable is the legacy shared outbox table (product, order and payment all wrote to it, ADR-23).
// Every service now sets its own table with SetTable (ADR-34); the default only serves tests and old callers.
const DefaultTable = "domain_events"

var (
	tableMu sync.RWMutex
	table   = DefaultTable
	tableRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)
)

// SetTable chooses the outbox table this process emits to and publishes from (for example
// "order_domain_events"). Call it once at start-up, before Emit / Run / Backlog; the name is spliced into SQL,
// so anything that is not a plain lower-case identifier panics.
func SetTable(name string) {
	if !tableRe.MatchString(name) {
		panic(fmt.Sprintf("events: invalid outbox table name %q", name))
	}
	tableMu.Lock()
	table = name
	tableMu.Unlock()
}

// Table returns the outbox table currently in use.
func Table() string {
	tableMu.RLock()
	defer tableMu.RUnlock()
	return table
}

// Event is one outbox row (table Table(), one per service).
type Event struct {
	ID          uint64         `gorm:"primaryKey;autoIncrement"`
	Topic       string         `gorm:"not null"`
	Key         string         `gorm:"not null"`
	Payload     datatypes.JSON `gorm:"not null"`
	Status      string         `gorm:"not null;default:'PENDING';index:idx_domain_events_status_id,priority:1"`
	Attempts    int            `gorm:"not null;default:0"`
	CreatedAt   time.Time      `gorm:"not null;autoCreateTime"`
	PublishedAt *time.Time
	// NextAttemptAt: the row is not tried before this instant (exponential backoff after a failure).
	NextAttemptAt time.Time `gorm:"not null"`
	LastError     *string
	DeadAt        *time.Time
	// RequestID is the X-Request-ID of the request that caused the event (ADR-36); the publisher sends it as the
	// Kafka header "x-request-id". NULL for rows written before the column existed or outside a request.
	RequestID *string `gorm:"column:request_id"`
	// AgentRunID is set when the action was confirmed by the user from an AI agent proposal (ADR-42): the id of that chat
	// run. NULL = the user acted directly (or the row predates the column).
	AgentRunID *string `gorm:"column:agent_run_id"`
}

// TableName is the service's own outbox table (see SetTable).
func (Event) TableName() string { return Table() }

// Emit stores an event in the caller's transaction (pass the *gorm.DB of the transaction).
// payload is JSON-encoded; a "v" (version) field is advisable in payload types.
func Emit(tx *gorm.DB, topic, key string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e := &Event{Topic: topic, Key: key, Payload: b, Status: StatusPending, NextAttemptAt: time.Now()}
	// tx.Statement.Context is the ctx the transaction was opened with (db.WithContext(ctx).Transaction ...).
	if id := reqctx.Get(tx.Statement.Context); id != "" {
		e.RequestID = &id
	}
	if id := reqctx.AgentRun(tx.Statement.Context); id != "" {
		e.AgentRunID = &id
	}
	return tx.Table(Table()).Create(e).Error
}

// Options tunes the failure handling of PublishBatch.
type Options struct {
	MaxAttempts int           // failed publishes before a row becomes DEAD (default 10)
	BackoffBase time.Duration // delay after the first failure, doubled on every further one (default 5s)
	BackoffMax  time.Duration // upper bound of the delay (default 15m)
}

// DefaultOptions are the values used when nothing is configured.
func DefaultOptions() Options {
	return Options{MaxAttempts: 10, BackoffBase: 5 * time.Second, BackoffMax: 15 * time.Minute}
}

// OptionsFromEnv reads OUTBOX_MAX_ATTEMPTS, OUTBOX_BACKOFF_BASE and OUTBOX_BACKOFF_MAX (Go durations, e.g. 5s);
// invalid or missing values keep the defaults.
func OptionsFromEnv() Options {
	o := DefaultOptions()
	if n, err := strconv.Atoi(os.Getenv("OUTBOX_MAX_ATTEMPTS")); err == nil && n > 0 {
		o.MaxAttempts = n
	}
	if d, err := time.ParseDuration(os.Getenv("OUTBOX_BACKOFF_BASE")); err == nil && d > 0 {
		o.BackoffBase = d
	}
	if d, err := time.ParseDuration(os.Getenv("OUTBOX_BACKOFF_MAX")); err == nil && d > 0 {
		o.BackoffMax = d
	}
	return o
}

// Defaults of PollFromEnv: how often RunMany looks for due rows and how many rows one batch takes.
const (
	DefaultPollInterval = 200 * time.Millisecond
	DefaultBatchSize    = 500
)

// PollFromEnv reads OUTBOX_POLL_INTERVAL (a Go duration, default 200ms) and OUTBOX_BATCH_SIZE (default 500) for RunMany.
func PollFromEnv() (time.Duration, int) {
	interval, batch := DefaultPollInterval, DefaultBatchSize
	if d, err := time.ParseDuration(os.Getenv("OUTBOX_POLL_INTERVAL")); err == nil && d > 0 {
		interval = d
	}
	if n, err := strconv.Atoi(os.Getenv("OUTBOX_BATCH_SIZE")); err == nil && n > 0 {
		batch = n
	}
	return interval, batch
}

func (o Options) withDefaults() Options {
	d := DefaultOptions()
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = d.MaxAttempts
	}
	if o.BackoffBase <= 0 {
		o.BackoffBase = d.BackoffBase
	}
	if o.BackoffMax <= 0 {
		o.BackoffMax = d.BackoffMax
	}
	return o
}

// backoff is the wait after the given number of failed attempts (1 = first failure).
func (o Options) backoff(attempts int) time.Duration {
	d := o.BackoffBase
	for i := 1; i < attempts && d < o.BackoffMax; i++ {
		d *= 2
	}
	if d > o.BackoffMax {
		d = o.BackoffMax
	}
	return d
}

// Process-wide counters (exported as mm_outbox_* by Register).
var published, publishFailures, deadMarked atomic.Int64

// PublishFunc sends one message to the broker.
type PublishFunc func(ctx context.Context, topic, key string, payload []byte) error

const (
	outageAfter = 3 // this many failures with zero successes in a batch = the broker is probably down
	abortAfter  = 5 // ... and the batch stops after this many (do not wait for every timeout)
)

// PublishBatch publishes up to limit due events (oldest first) with DefaultOptions; see PublishBatchOpts.
func PublishBatch(ctx context.Context, db *gorm.DB, publish PublishFunc, limit int, perMessage time.Duration) (int, error) {
	return PublishBatchOpts(ctx, db, publish, limit, perMessage, DefaultOptions())
}

// PublishBatchOpts publishes up to limit PENDING/FAILED events whose next_attempt_at has passed (oldest first)
// and returns how many were sent plus the first publish error, if any. Rows are locked with SKIP LOCKED so
// several replicas can run the worker.
//
// Failures are handled per row: a failing row gets attempts+1, last_error and a next_attempt_at with
// exponential backoff, and after opt.MaxAttempts it becomes DEAD (dead-letter, kept, never retried
// automatically). Ordering is kept per key (the aggregate id): a row is not published while an EARLIER row
// with the same key is waiting for its retry, but other keys go on. DEAD rows do not block later rows.
// When a whole batch fails (>= 3 failures, no success) the broker is assumed down: attempts still grow and
// backoff applies, but nothing is declared DEAD, so an outage cannot dead-letter healthy events.
func PublishBatchOpts(ctx context.Context, db *gorm.DB, publish PublishFunc, limit int, perMessage time.Duration, opt Options) (int, error) {
	opt = opt.withDefaults()
	sent := 0
	var firstErr error
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		tbl := Table()
		unpublished := []string{StatusPending, StatusFailed}
		var rows []Event
		if err := tx.Table(tbl).Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND next_attempt_at <= ?", unpublished, now).
			Where("NOT EXISTS (SELECT 1 FROM "+tbl+" p WHERE p.key = "+tbl+".key AND p.id < "+tbl+".id AND p.status IN ? AND p.next_attempt_at > ?)", unpublished, now).
			Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		type failure struct {
			e   *Event
			err error
		}
		var failed []failure
		blocked := map[string]bool{} // keys with a failure in this batch: later rows of the key wait
		for i := range rows {
			e := &rows[i]
			if blocked[e.Key] {
				continue
			}
			if len(failed) >= abortAfter && sent == 0 {
				break
			}
			mctx, cancel := context.WithTimeout(ctx, perMessage)
			if e.RequestID != nil {
				mctx = reqctx.Put(mctx, *e.RequestID) // the producer turns it into the Kafka header
			}
			err := publish(mctx, e.Topic, e.Key, e.Payload)
			cancel()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				blocked[e.Key] = true
				failed = append(failed, failure{e, err})
				continue
			}
			if err := tx.Table(tbl).Model(e).Updates(map[string]any{"status": StatusDone, "published_at": time.Now(), "last_error": nil}).Error; err != nil {
				return err
			}
			sent++
		}
		outage := sent == 0 && len(failed) >= outageAfter
		for _, f := range failed {
			attempts := f.e.Attempts + 1
			msg := f.err.Error()
			if len(msg) > 1000 {
				msg = msg[:1000]
			}
			upd := map[string]any{"attempts": attempts, "last_error": msg, "status": StatusFailed,
				"next_attempt_at": now.Add(opt.backoff(attempts))}
			if attempts >= opt.MaxAttempts && !outage {
				upd["status"] = StatusDead
				upd["dead_at"] = now
				deadMarked.Add(1)
			}
			if err := tx.Table(tbl).Model(f.e).Updates(upd).Error; err != nil {
				return err
			}
		}
		publishFailures.Add(int64(len(failed)))
		return nil
	})
	if err != nil {
		return 0, err
	}
	published.Add(int64(sent))
	return sent, firstErr
}

// Message is one outbox row handed to a batch publisher. It lives in kafkaimpl so that kafkaimpl (used by services
// without Postgres, e.g. the gateway) does not import this package and, through it, gorm.
type Message = kafkaimpl.Message

// PublishManyFunc sends msgs together (one round trip per topic) and returns one error per message, in order
// (nil = delivered); len(result) must equal len(msgs).
type PublishManyFunc func(ctx context.Context, msgs []Message) []error

// PublishBatchMany is PublishBatchOpts with a batching publisher. Publishing one message at a time costs a full Kafka
// round trip (acks from all replicas plus the writer's batch timeout) per row, which capped one service at a few dozen
// events per second; here a whole batch costs one round trip per topic and one UPDATE.
//
// Per-key ordering is kept by sending only the OLDEST due row of each key in a batch: later rows of the same key wait for
// the next batch (RunMany runs the next one straight away) and, if that row fails, stay blocked by the backoff rule in the
// query until it is retried. Failure handling (backoff, DEAD after MaxAttempts, broker outage) is the same as PublishBatchOpts.
func PublishBatchMany(ctx context.Context, db *gorm.DB, publish PublishManyFunc, limit int, timeout time.Duration, opt Options) (int, error) {
	opt = opt.withDefaults()
	sent := 0
	var firstErr error
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		tbl := Table()
		unpublished := []string{StatusPending, StatusFailed}
		var rows []Event
		if err := tx.Table(tbl).Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND next_attempt_at <= ?", unpublished, now).
			Where("NOT EXISTS (SELECT 1 FROM "+tbl+" p WHERE p.key = "+tbl+".key AND p.id < "+tbl+".id AND p.status IN ? AND p.next_attempt_at > ?)", unpublished, now).
			Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		batch := make([]*Event, 0, len(rows))
		seen := map[string]bool{}
		for i := range rows {
			if seen[rows[i].Key] {
				continue // a newer row of a key already in this batch waits for the next batch
			}
			seen[rows[i].Key] = true
			batch = append(batch, &rows[i])
		}
		if len(batch) == 0 {
			return nil
		}
		msgs := make([]Message, len(batch))
		for i, e := range batch {
			msgs[i] = Message{Topic: e.Topic, Key: e.Key, Payload: e.Payload, RequestID: e.RequestID}
		}
		mctx, cancel := context.WithTimeout(ctx, timeout)
		errs := publish(mctx, msgs)
		cancel()
		if len(errs) != len(msgs) { // a misbehaving publisher must not mark anything as sent
			e := fmt.Errorf("events: publisher returned %d results for %d messages", len(errs), len(msgs))
			errs = make([]error, len(msgs))
			for i := range errs {
				errs[i] = e
			}
		}
		var doneIDs []uint64
		type failure struct {
			e   *Event
			err error
		}
		var failed []failure
		for i, e := range batch {
			if errs[i] != nil {
				if firstErr == nil {
					firstErr = errs[i]
				}
				failed = append(failed, failure{e, errs[i]})
				continue
			}
			doneIDs = append(doneIDs, e.ID)
		}
		if len(doneIDs) > 0 {
			if err := tx.Table(tbl).Where("id IN ?", doneIDs).Updates(map[string]any{"status": StatusDone, "published_at": time.Now(), "last_error": nil}).Error; err != nil {
				return err
			}
			sent = len(doneIDs)
		}
		outage := sent == 0 && len(failed) >= outageAfter
		for _, f := range failed {
			attempts := f.e.Attempts + 1
			msg := f.err.Error()
			if len(msg) > 1000 {
				msg = msg[:1000]
			}
			upd := map[string]any{"attempts": attempts, "last_error": msg, "status": StatusFailed,
				"next_attempt_at": now.Add(opt.backoff(attempts))}
			if attempts >= opt.MaxAttempts && !outage {
				upd["status"] = StatusDead
				upd["dead_at"] = now
				deadMarked.Add(1)
			}
			if err := tx.Table(tbl).Model(f.e).Updates(upd).Error; err != nil {
				return err
			}
		}
		publishFailures.Add(int64(len(failed)))
		return nil
	})
	if err != nil {
		return 0, err
	}
	published.Add(int64(sent))
	return sent, firstErr
}

// RunMany is Run with a batching publisher: every interval it publishes batches until one makes no progress (so a backlog
// drains at the speed of Kafka instead of one tick per batch), at most maxDrain batches per tick so a poisoned table cannot
// spin forever. Prefer a short interval (a few hundred ms): an idle tick is one cheap indexed SELECT.
func RunMany(ctx context.Context, db *gorm.DB, publish PublishManyFunc, interval time.Duration, limit int, onError func(error)) {
	const maxDrain = 50
	opt := OptionsFromEnv()
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				for i := 0; i < maxDrain && ctx.Err() == nil; i++ {
					n, err := PublishBatchMany(ctx, db, publish, limit, 10*time.Second, opt)
					if err != nil && onError != nil {
						onError(err)
					}
					if n == 0 || err != nil {
						break
					}
				}
			}
		}
	}()
}

// Run calls PublishBatchOpts every interval until ctx is done, with OptionsFromEnv. onError may be nil.
func Run(ctx context.Context, db *gorm.DB, publish PublishFunc, interval time.Duration, limit int, onError func(error)) {
	opt := OptionsFromEnv()
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := PublishBatchOpts(ctx, db, publish, limit, 5*time.Second, opt); err != nil && onError != nil {
					onError(err)
				}
			}
		}
	}()
}

// Register exports the outbox metrics on reg (service becomes a constant label):
// mm_outbox_published_total, mm_outbox_publish_failures_total, mm_outbox_dead_total (since process start)
// and mm_outbox_dead, the number of DEAD rows in the table (needs attention, see the runbook).
func Register(reg prometheus.Registerer, service string, db *gorm.DB) {
	l := prometheus.Labels{"service": service, "table": Table()}
	reg.MustRegister(
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "mm_outbox_published_total", Help: "Outbox rows published to Kafka by this process.", ConstLabels: l}, func() float64 { return float64(published.Load()) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "mm_outbox_publish_failures_total", Help: "Failed outbox publish attempts by this process.", ConstLabels: l}, func() float64 { return float64(publishFailures.Load()) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "mm_outbox_dead_total", Help: "Outbox rows dead-lettered by this process.", ConstLabels: l}, func() float64 { return float64(deadMarked.Load()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "mm_outbox_oldest_pending_seconds", Help: "Age of the oldest outbox row still waiting to be published (0 = none): the publish lag.", ConstLabels: l}, func() float64 {
			return OldestPendingSeconds(db)
		}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "mm_outbox_dead", Help: "Outbox rows in the DEAD (dead-letter) state.", ConstLabels: l}, func() float64 {
			n, _ := DeadCount(db)
			return float64(n)
		}),
	)
}

// OldestPendingSeconds is the age in seconds of the oldest row that is still PENDING or FAILED (0 when none or on error).
func OldestPendingSeconds(db *gorm.DB) float64 {
	var age *float64
	if err := db.Raw("SELECT EXTRACT(EPOCH FROM (now() - min(created_at)))::float8 FROM "+Table()+" WHERE status IN ?", []string{StatusPending, StatusFailed}).Scan(&age).Error; err != nil || age == nil || *age < 0 {
		return 0
	}
	return *age
}

// DeadCount returns the number of dead-lettered rows.
func DeadCount(db *gorm.DB) (int64, error) {
	var n int64
	err := db.Table(Table()).Model(&Event{}).Where("status = ?", StatusDead).Count(&n).Error
	return n, err
}

// Backlog returns the number of unpublished events and the age in seconds of the oldest one.
func Backlog(db *gorm.DB) (pending int64, oldestSeconds float64, err error) {
	var row struct {
		N      int64
		Oldest *time.Time
	}
	err = db.Table(Table()).Model(&Event{}).Select("count(*) AS n, min(created_at) AS oldest").
		Where("status IN ?", []string{StatusPending, StatusFailed}).Scan(&row).Error
	if err != nil || row.Oldest == nil {
		return row.N, 0, err
	}
	return row.N, time.Since(*row.Oldest).Seconds(), nil
}

// DeadRow is a dead-lettered outbox row without its payload body in the listing (use the id to inspect it).
type DeadRow struct {
	ID        uint64     `json:"id"`
	Topic     string     `json:"topic"`
	Key       string     `json:"key"`
	Attempts  int        `json:"attempts"`
	LastError *string    `json:"last_error,omitempty"`
	DeadAt    *time.Time `json:"dead_at,omitempty"`
	RequestID *string    `json:"request_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ListDead returns up to limit dead-lettered rows, newest first (limit <= 0 means 50, capped at 500).
func ListDead(db *gorm.DB, limit int) ([]DeadRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	var rows []DeadRow
	err := db.Table(Table()).Select("id, topic, key, attempts, last_error, dead_at, request_id, created_at").
		Where("status = ?", StatusDead).Order("id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// RequeueDead puts dead-lettered rows back in the queue (status FAILED, attempts 0, due now) so the worker
// publishes them again, and returns how many it changed. ids selects rows; an empty ids requeues every DEAD
// row up to limit (<= 0 means 100). Only DEAD rows are touched. Consumers are idempotent (at-least-once), so a
// repeat is safe; fix the cause first (see last_error), otherwise the row dies again after MaxAttempts.
func RequeueDead(db *gorm.DB, ids []uint64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	q := db.Table(Table()).Where("status = ?", StatusDead)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	} else {
		sub := db.Table(Table()).Select("id").Where("status = ?", StatusDead).Order("id ASC").Limit(limit)
		q = q.Where("id IN (?)", sub)
	}
	res := q.Updates(map[string]any{"status": StatusFailed, "attempts": 0, "next_attempt_at": time.Now(), "dead_at": nil})
	return res.RowsAffected, res.Error
}
