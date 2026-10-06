// Package events is a tiny transactional outbox shared by the services (an identical copy lives in each
// service module, see ADR-8). A business change calls Emit inside its own database transaction; a worker
// (Run / PublishBatch) later publishes the rows to Kafka. Delivery is at-least-once, so consumers must be
// idempotent (the key and payload carry natural ids for that).
package events

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StatusPending = "PENDING"
	StatusFailed  = "FAILED"
	StatusDone    = "SUCCESS"
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
}

// TableName keeps the table name stable.
func (Event) TableName() string { return "domain_events" }

// Migrate creates the outbox table.
func Migrate(db *gorm.DB) error { return db.AutoMigrate(&Event{}) }

// Emit stores an event in the caller's transaction (pass the *gorm.DB of the transaction).
// payload is JSON-encoded; a "v" (version) field is advisable in payload types.
func Emit(tx *gorm.DB, topic, key string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Create(&Event{Topic: topic, Key: key, Payload: b, Status: StatusPending}).Error
}

// PublishFunc sends one message to the broker.
type PublishFunc func(ctx context.Context, topic, key string, payload []byte) error

// PublishBatch publishes up to limit pending/failed events (oldest first) and returns how many were sent.
// Rows are locked with SKIP LOCKED so several replicas can run the worker. The first publish error stops
// the batch (keeping order); rows sent before it are still marked done and the error is returned.
func PublishBatch(ctx context.Context, db *gorm.DB, publish PublishFunc, limit int, perMessage time.Duration) (int, error) {
	sent := 0
	var publishErr error
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []Event
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ?", []string{StatusPending, StatusFailed}).
			Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			e := &rows[i]
			mctx, cancel := context.WithTimeout(ctx, perMessage)
			err := publish(mctx, e.Topic, e.Key, e.Payload)
			cancel()
			if err != nil {
				publishErr = err
				return tx.Model(e).Updates(map[string]any{"status": StatusFailed, "attempts": gorm.Expr("attempts + 1")}).Error
			}
			if err := tx.Model(e).Updates(map[string]any{"status": StatusDone, "published_at": time.Now()}).Error; err != nil {
				return err
			}
			sent++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return sent, publishErr
}

// Run calls PublishBatch every interval until ctx is done. onError may be nil.
func Run(ctx context.Context, db *gorm.DB, publish PublishFunc, interval time.Duration, limit int, onError func(error)) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := PublishBatch(ctx, db, publish, limit, 5*time.Second); err != nil && onError != nil {
					onError(err)
				}
			}
		}
	}()
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
