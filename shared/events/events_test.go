package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The production schema is created by each service's goose migrations; the tests only need the table that
// Event maps to, so it is created here (columns and the pending-rows index mirror the migrations).
const ddl = `
CREATE TABLE domain_events (
  id BIGSERIAL PRIMARY KEY,
  topic TEXT NOT NULL,
  key TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING',
  attempts INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error TEXT,
  dead_at TIMESTAMPTZ
);
CREATE INDEX idx_domain_events_unpublished ON domain_events (id) WHERE status IN ('PENDING', 'FAILED');`

// testDB creates a throw-away database on TEST_POSTGRES_DSN (skipped when unset) with the domain_events table.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("outbox_%d", rand.Int63())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	sdb, err := sql.Open("pgx", dsn+" dbname="+name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sdb.Close()
		admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		admin.Close()
	})
	if _, err := sdb.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sdb}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func emit(t *testing.T, db *gorm.DB, key string) {
	t.Helper()
	if err := Emit(db, "t.topic", key, map[string]any{"k": key}); err != nil {
		t.Fatal(err)
	}
}

// failFor fails publishes for the given keys (recording every attempt) and accepts the rest.
func failFor(boom error, tried *[]string, keys ...string) PublishFunc {
	bad := map[string]bool{}
	for _, k := range keys {
		bad[k] = true
	}
	return func(_ context.Context, _, key string, _ []byte) error {
		*tried = append(*tried, key)
		if bad[key] {
			return boom
		}
		return nil
	}
}

func rowByKey(t *testing.T, db *gorm.DB, key string) Event {
	t.Helper()
	var e Event
	if err := db.Where("key = ?", key).Order("id").First(&e).Error; err != nil {
		t.Fatal(err)
	}
	return e
}

// due makes every unpublished row eligible again (skips the backoff wait).
func due(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Model(&Event{}).Where("status IN ?", []string{StatusPending, StatusFailed}).
		Update("next_attempt_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPoisonRowDoesNotBlockOtherKeys(t *testing.T) {
	db := testDB(t)
	emit(t, db, "bad")
	emit(t, db, "good1")
	emit(t, db, "good2")
	var tried []string
	sent, err := PublishBatch(context.Background(), db, failFor(errors.New("boom"), &tried, "bad"), 10, time.Second)
	if sent != 2 || err == nil {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
	if e := rowByKey(t, db, "bad"); e.Status != StatusFailed || e.Attempts != 1 || e.LastError == nil || !e.NextAttemptAt.After(time.Now()) {
		t.Fatalf("bad row: %+v", e)
	}
	for _, k := range []string{"good1", "good2"} {
		if e := rowByKey(t, db, k); e.Status != StatusDone || e.PublishedAt == nil {
			t.Fatalf("%s: %+v", k, e)
		}
	}
}

func TestSameKeyKeepsOrder(t *testing.T) {
	db := testDB(t)
	emit(t, db, "a") // first event of key a fails ...
	emit(t, db, "a") // ... so the second must wait
	emit(t, db, "b")
	var tried []string
	fail := true
	pub := func(ctx context.Context, topic, key string, p []byte) error {
		tried = append(tried, key)
		if key == "a" && fail {
			return errors.New("boom")
		}
		return nil
	}
	if sent, _ := PublishBatch(context.Background(), db, pub, 10, time.Second); sent != 1 {
		t.Fatalf("sent=%d", sent)
	}
	if n := len(tried); n != 2 { // a (failed), b; the second a was not attempted
		t.Fatalf("tried=%v", tried)
	}
	// still backing off: a later batch must not publish the second a
	tried = nil
	fail = false
	if sent, _ := PublishBatch(context.Background(), db, pub, 10, time.Second); sent != 0 || len(tried) != 0 {
		t.Fatalf("sent=%d tried=%v", sent, tried)
	}
	due(t, db)
	if sent, err := PublishBatch(context.Background(), db, pub, 10, time.Second); sent != 2 || err != nil {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
}

func TestDeadAfterMaxAttemptsAndRequeue(t *testing.T) {
	db := testDB(t)
	emit(t, db, "bad")
	emit(t, db, "ok")
	opt := Options{MaxAttempts: 2, BackoffBase: time.Millisecond, BackoffMax: time.Millisecond}
	var tried []string
	pub := failFor(errors.New("boom"), &tried, "bad")
	for i := 0; i < 2; i++ {
		PublishBatchOpts(context.Background(), db, pub, 10, time.Second, opt)
		due(t, db)
	}
	e := rowByKey(t, db, "bad")
	if e.Status != StatusDead || e.Attempts != 2 || e.DeadAt == nil {
		t.Fatalf("bad row: %+v", e)
	}
	if n, _ := DeadCount(db); n != 1 {
		t.Fatalf("dead=%d", n)
	}
	// a DEAD row is never retried automatically
	tried = nil
	PublishBatchOpts(context.Background(), db, pub, 10, time.Second, opt)
	if len(tried) != 0 {
		t.Fatalf("dead row retried: %v", tried)
	}
	// manual requeue (runbook) makes it deliverable again
	if err := db.Model(&Event{}).Where("status = ?", StatusDead).
		Updates(map[string]any{"status": StatusPending, "attempts": 0, "dead_at": nil, "next_attempt_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if sent, err := PublishBatchOpts(context.Background(), db, func(context.Context, string, string, []byte) error { return nil }, 10, time.Second, opt); sent != 1 || err != nil {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
}

func TestBrokerOutageDoesNotDeadLetter(t *testing.T) {
	db := testDB(t)
	for _, k := range []string{"a", "b", "c"} {
		emit(t, db, k)
	}
	opt := Options{MaxAttempts: 1, BackoffBase: time.Millisecond, BackoffMax: time.Millisecond}
	down := func(context.Context, string, string, []byte) error { return errors.New("broker down") }
	if sent, err := PublishBatchOpts(context.Background(), db, down, 10, time.Second, opt); sent != 0 || err == nil {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
	if n, _ := DeadCount(db); n != 0 {
		t.Fatalf("outage dead-lettered %d rows", n)
	}
	var cnt int64
	db.Model(&Event{}).Where("status = ?", StatusFailed).Count(&cnt)
	if cnt != 3 {
		t.Fatalf("failed=%d", cnt)
	}
}

func TestBackoffAndOptionsFromEnv(t *testing.T) {
	o := Options{BackoffBase: time.Second, BackoffMax: 5 * time.Second}
	for attempts, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 5 * time.Second, 9: 5 * time.Second} {
		if got := o.backoff(attempts); got != want {
			t.Errorf("backoff(%d)=%v want %v", attempts, got, want)
		}
	}
	t.Setenv("OUTBOX_MAX_ATTEMPTS", "3")
	t.Setenv("OUTBOX_BACKOFF_BASE", "250ms")
	t.Setenv("OUTBOX_BACKOFF_MAX", "bogus")
	got := OptionsFromEnv()
	if got.MaxAttempts != 3 || got.BackoffBase != 250*time.Millisecond || got.BackoffMax != DefaultOptions().BackoffMax {
		t.Fatalf("%+v", got)
	}
}
