// Package events is a tiny transactional outbox shared by the services (module mini-marketplace/shared, ADR-33).
// A business change calls Emit inside its own database transaction; a worker
// (Run / PublishBatch) later publishes the rows to Kafka. Delivery is at-least-once, so consumers must be
// idempotent (the key and payload carry natural ids for that).
package events

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StatusPending = "PENDING"
	StatusFailed  = "FAILED"
	StatusDone    = "SUCCESS"
	// StatusDead is the dead-letter state: the row failed Options.MaxAttempts times, is kept for inspection
	// and is NOT retried automatically (requeue by hand, see docs/instruction/03_RUNBOOK.md section 5).
	StatusDead = "DEAD"
)

// Event is one outbox row (table domain_events).
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
}

// TableName keeps the table name stable.
func (Event) TableName() string { return "domain_events" }

// Emit stores an event in the caller's transaction (pass the *gorm.DB of the transaction).
// payload is JSON-encoded; a "v" (version) field is advisable in payload types.
func Emit(tx *gorm.DB, topic, key string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Create(&Event{Topic: topic, Key: key, Payload: b, Status: StatusPending, NextAttemptAt: time.Now()}).Error
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
		unpublished := []string{StatusPending, StatusFailed}
		var rows []Event
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND next_attempt_at <= ?", unpublished, now).
			Where("NOT EXISTS (SELECT 1 FROM domain_events p WHERE p.key = domain_events.key AND p.id < domain_events.id AND p.status IN ? AND p.next_attempt_at > ?)", unpublished, now).
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
			if err := tx.Model(e).Updates(map[string]any{"status": StatusDone, "published_at": time.Now(), "last_error": nil}).Error; err != nil {
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
			if err := tx.Model(f.e).Updates(upd).Error; err != nil {
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
	l := prometheus.Labels{"service": service, "table": "domain_events"}
	reg.MustRegister(
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "mm_outbox_published_total", Help: "Outbox rows published to Kafka by this process.", ConstLabels: l}, func() float64 { return float64(published.Load()) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "mm_outbox_publish_failures_total", Help: "Failed outbox publish attempts by this process.", ConstLabels: l}, func() float64 { return float64(publishFailures.Load()) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: "mm_outbox_dead_total", Help: "Outbox rows dead-lettered by this process.", ConstLabels: l}, func() float64 { return float64(deadMarked.Load()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "mm_outbox_dead", Help: "Outbox rows in the DEAD (dead-letter) state.", ConstLabels: l}, func() float64 {
			n, _ := DeadCount(db)
			return float64(n)
		}),
	)
}

// DeadCount returns the number of dead-lettered rows.
func DeadCount(db *gorm.DB) (int64, error) {
	var n int64
	err := db.Model(&Event{}).Where("status = ?", StatusDead).Count(&n).Error
	return n, err
}

// Backlog returns the number of unpublished events and the age in seconds of the oldest one.
func Backlog(db *gorm.DB) (pending int64, oldestSeconds float64, err error) {
	var row struct {
		N      int64
		Oldest *time.Time
	}
	err = db.Model(&Event{}).Select("count(*) AS n, min(created_at) AS oldest").
		Where("status IN ?", []string{StatusPending, StatusFailed}).Scan(&row).Error
	if err != nil || row.Oldest == nil {
		return row.N, 0, err
	}
	return row.N, time.Since(*row.Oldest).Seconds(), nil
}
