package events

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"mini-marketplace/shared/reqctx"
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
  dead_at TIMESTAMPTZ,
  request_id TEXT,
  agent_run_id TEXT
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

func TestSetTableValidatesAndSeparatesOutboxes(t *testing.T) {
	defer SetTable(DefaultTable)
	for _, bad := range []string{"", "x; DROP TABLE y", "Upper", "a-b", "1abc"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("SetTable(%q) did not panic", bad)
				}
			}()
			SetTable(bad)
		}()
	}
	db := testDB(t)
	if err := db.Exec(`CREATE TABLE order_domain_events (LIKE domain_events INCLUDING ALL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO domain_events (topic, key, payload) VALUES ('t', 'legacy', '{}')`).Error; err != nil {
		t.Fatal(err)
	}
	SetTable("order_domain_events")
	if Table() != "order_domain_events" {
		t.Fatalf("Table() = %s", Table())
	}
	emit(t, db, "mine")
	if n, _, _ := Backlog(db); n != 1 {
		t.Errorf("backlog of own table = %d, want 1 (legacy row must not count)", n)
	}
	var tried []string
	if sent, err := PublishBatch(context.Background(), db, failFor(nil, &tried), 10, time.Second); sent != 1 || err != nil || len(tried) != 1 || tried[0] != "mine" {
		t.Errorf("sent=%d err=%v tried=%v", sent, err, tried)
	}
}

type rkey struct{}

func TestRequestIDStoredAndPassedToPublisher(t *testing.T) {
	reqctx.Use(func(c context.Context) string { s, _ := c.Value(rkey{}).(string); return s },
		func(c context.Context, id string) context.Context { return context.WithValue(c, rkey{}, id) })
	t.Cleanup(func() { reqctx.Use(nil, nil) })
	db := testDB(t)
	ctx := context.WithValue(context.Background(), rkey{}, "req-abcdef123")
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return Emit(tx, "t", "with", map[string]int{"v": 1}) }); err != nil {
		t.Fatal(err)
	}
	emit(t, db, "without") // no request id in ctx
	if e := rowByKey(t, db, "with"); e.RequestID == nil || *e.RequestID != "req-abcdef123" {
		t.Fatalf("request_id not stored: %+v", e)
	}
	if e := rowByKey(t, db, "without"); e.RequestID != nil {
		t.Fatalf("request_id should be NULL: %v", *e.RequestID)
	}
	got := map[string]string{}
	pub := func(c context.Context, _ string, key string, _ []byte) error {
		got[key], _ = c.Value(rkey{}).(string)
		return nil
	}
	if sent, err := PublishBatch(context.Background(), db, pub, 10, time.Second); sent != 2 || err != nil {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
	if got["with"] != "req-abcdef123" || got["without"] != "" {
		t.Fatalf("ctx passed to publisher: %v", got)
	}
}

func TestListAndRequeueDead(t *testing.T) {
	db := testDB(t)
	for _, k := range []string{"a", "b", "c"} {
		emit(t, db, k)
	}
	if err := db.Model(&Event{}).Where("key IN ?", []string{"a", "b"}).
		Updates(map[string]any{"status": StatusDead, "attempts": 10, "dead_at": time.Now(), "last_error": "boom"}).Error; err != nil {
		t.Fatal(err)
	}
	rows, err := ListDead(db, 0)
	if err != nil || len(rows) != 2 || rows[0].Key != "b" || rows[0].LastError == nil {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	// by id: only that row; a non-DEAD id is ignored
	idC := rowByKey(t, db, "c").ID
	n, err := RequeueDead(db, []uint64{rows[1].ID, idC}, 0)
	if err != nil || n != 1 {
		t.Fatalf("requeue by id n=%d err=%v", n, err)
	}
	if e := rowByKey(t, db, "a"); e.Status != StatusFailed || e.Attempts != 0 || e.DeadAt != nil {
		t.Fatalf("a not requeued: %+v", e)
	}
	if e := rowByKey(t, db, "b"); e.Status != StatusDead {
		t.Fatalf("b must stay DEAD: %+v", e)
	}
	// no ids: every remaining DEAD row
	if n, err := RequeueDead(db, nil, 10); err != nil || n != 1 {
		t.Fatalf("requeue all n=%d err=%v", n, err)
	}
	if n, _ := DeadCount(db); n != 0 {
		t.Fatalf("dead=%d", n)
	}
	if sent, err := PublishBatch(context.Background(), db, func(context.Context, string, string, []byte) error { return nil }, 10, time.Second); sent != 3 || err != nil {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
}

func TestAdminMountListAndRequeue(t *testing.T) {
	db := testDB(t)
	emit(t, db, "x")
	emit(t, db, "y")
	db.Model(&Event{}).Where("key = ?", "x").Updates(map[string]any{"status": StatusDead, "attempts": 10, "dead_at": time.Now(), "last_error": "boom"})
	mux := http.NewServeMux()
	Mount(db)(mux)
	do := func(method, url string, hdr ...string) (int, string) {
		req := httptest.NewRequest(method, url, nil)
		if len(hdr) == 1 {
			req.Header.Set("Authorization", hdr[0])
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	if code, body := do("GET", "/internal/outbox/dead"); code != 200 || !strings.Contains(body, `"dead_total":1`) || !strings.Contains(body, "boom") {
		t.Fatalf("list: %d %s", code, body)
	}
	if code, _ := do("POST", "/internal/outbox/requeue?id=abc"); code != 400 {
		t.Fatalf("bad id: %d", code)
	}
	if code, body := do("POST", "/internal/outbox/requeue"); code != 200 || !strings.Contains(body, `"requeued":1`) {
		t.Fatalf("requeue: %d %s", code, body)
	}
	if n, _ := DeadCount(db); n != 0 {
		t.Fatalf("dead=%d", n)
	}
}

func TestAdminMountRequiresTokenWhenConfigured(t *testing.T) {
	t.Setenv("METRICS_TOKEN", "s3cret-token")
	db := testDB(t)
	mux := http.NewServeMux()
	Mount(db)(mux)
	for hdr, want := range map[string]int{"": 401, "Bearer wrong": 401, "Bearer s3cret-token": 200} {
		req := httptest.NewRequest("GET", "/internal/outbox/dead", nil)
		if hdr != "" {
			req.Header.Set("Authorization", hdr)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("auth %q: %d want %d", hdr, rec.Code, want)
		}
	}
}

func TestAgentRunIDStoredOnlyWhenConfirmedFromAgent(t *testing.T) {
	db := testDB(t)
	ctx := reqctx.WithAgentRun(context.Background(), "run-0123456789")
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return Emit(tx, "t", "agent", map[string]int{"v": 1}) }); err != nil {
		t.Fatal(err)
	}
	emit(t, db, "direct") // the user acted directly
	if e := rowByKey(t, db, "agent"); e.AgentRunID == nil || *e.AgentRunID != "run-0123456789" {
		t.Fatalf("agent_run_id not stored: %+v", e)
	}
	if e := rowByKey(t, db, "direct"); e.AgentRunID != nil {
		t.Fatalf("direct action must have NULL agent_run_id, got %v", *e.AgentRunID)
	}
}

// ---- batching publisher (PublishBatchMany / RunMany)

// manyRec records every call of a batch publisher; fail lists keys whose message is refused.
type manyRec struct {
	calls [][]string // keys per call, in order
	fail  map[string]error
}

func (r *manyRec) publish(_ context.Context, msgs []Message) []error {
	keys := make([]string, len(msgs))
	errs := make([]error, len(msgs))
	for i, m := range msgs {
		keys[i] = m.Key
		errs[i] = r.fail[m.Key]
	}
	r.calls = append(r.calls, keys)
	return errs
}

func TestBatchManySendsAllKeysInOneCallAndMarksThemDone(t *testing.T) {
	db := testDB(t)
	for _, k := range []string{"a", "b", "c"} {
		emit(t, db, k)
	}
	r := &manyRec{}
	sent, err := PublishBatchMany(context.Background(), db, r.publish, 10, time.Second, Options{})
	if sent != 3 || err != nil || len(r.calls) != 1 || len(r.calls[0]) != 3 {
		t.Fatalf("sent=%d err=%v calls=%v", sent, err, r.calls)
	}
	for _, k := range []string{"a", "b", "c"} {
		if e := rowByKey(t, db, k); e.Status != StatusDone || e.PublishedAt == nil {
			t.Fatalf("%s: %+v", k, e)
		}
	}
}

func TestBatchManySendsOnlyTheOldestRowOfAKeyPerBatch(t *testing.T) {
	db := testDB(t)
	emit(t, db, "a")
	emit(t, db, "a")
	emit(t, db, "b")
	r := &manyRec{}
	if sent, _ := PublishBatchMany(context.Background(), db, r.publish, 10, time.Second, Options{}); sent != 2 || len(r.calls[0]) != 2 {
		t.Fatalf("first batch: sent=%d calls=%v", sent, r.calls)
	}
	if sent, _ := PublishBatchMany(context.Background(), db, r.publish, 10, time.Second, Options{}); sent != 1 || len(r.calls) != 2 { // the second a, next batch
		t.Fatalf("second batch: sent=%d calls=%v", sent, r.calls)
	}
}

func TestBatchManyPartialFailureBacksOffOnlyTheFailedRowAndBlocksItsKey(t *testing.T) {
	db := testDB(t)
	emit(t, db, "bad")
	emit(t, db, "bad") // must wait for the first one
	emit(t, db, "good")
	r := &manyRec{fail: map[string]error{"bad": errors.New("boom")}}
	sent, err := PublishBatchMany(context.Background(), db, r.publish, 10, time.Second, Options{})
	if sent != 1 || err == nil {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
	if e := rowByKey(t, db, "bad"); e.Status != StatusFailed || e.Attempts != 1 || !e.NextAttemptAt.After(time.Now()) {
		t.Fatalf("bad row: %+v", e)
	}
	if e := rowByKey(t, db, "good"); e.Status != StatusDone {
		t.Fatalf("good row: %+v", e)
	}
	r.calls = nil
	if sent, _ := PublishBatchMany(context.Background(), db, r.publish, 10, time.Second, Options{}); sent != 0 || len(r.calls) != 0 { // still backing off: the second bad must not jump the queue
		t.Fatalf("sent=%d calls=%v", sent, r.calls)
	}
	r.fail = nil
	due(t, db)
	if sent, err := PublishBatchMany(context.Background(), db, r.publish, 10, time.Second, Options{}); sent != 1 || err != nil {
		t.Fatalf("retry: sent=%d err=%v", sent, err)
	}
}

func TestBatchManyWrongResultCountMarksNothingAsSent(t *testing.T) {
	db := testDB(t)
	emit(t, db, "a")
	emit(t, db, "b")
	sent, err := PublishBatchMany(context.Background(), db, func(context.Context, []Message) []error { return nil }, 10, time.Second, Options{})
	if sent != 0 || err == nil {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
	if e := rowByKey(t, db, "a"); e.Status != StatusFailed {
		t.Fatalf("a: %+v", e)
	}
}

func TestBatchManyOutageDoesNotDeadLetter(t *testing.T) {
	db := testDB(t)
	for _, k := range []string{"a", "b", "c"} {
		emit(t, db, k)
	}
	r := &manyRec{fail: map[string]error{"a": errors.New("down"), "b": errors.New("down"), "c": errors.New("down")}}
	for i := 0; i < 3; i++ {
		PublishBatchMany(context.Background(), db, r.publish, 10, time.Second, Options{MaxAttempts: 2})
		due(t, db)
	}
	if e := rowByKey(t, db, "a"); e.Status == StatusDead {
		t.Fatalf("a dead-lettered during an outage: %+v", e)
	}
}

func TestRunManyDrainsABacklogWithinOneTickAndKeepsKeyOrder(t *testing.T) {
	db := testDB(t)
	for i := 0; i < 30; i++ {
		emit(t, db, fmt.Sprintf("k%d", i%10)) // 10 keys x 3 rows: needs at least 3 batches
	}
	var mu sync.Mutex
	order := map[string]int{}
	pub := func(_ context.Context, msgs []Message) []error {
		mu.Lock()
		defer mu.Unlock()
		for _, m := range msgs {
			order[m.Key]++
		}
		return make([]error, len(msgs))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	RunMany(ctx, db, pub, 20*time.Millisecond, 100, nil)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int64
		db.Model(&Event{}).Where("status = ?", StatusDone).Count(&n)
		if n == 30 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("backlog not drained: %v", order)
}

func TestOldestPendingSeconds(t *testing.T) {
	db := testDB(t)
	if v := OldestPendingSeconds(db); v != 0 {
		t.Fatalf("empty table: %v", v)
	}
	emit(t, db, "a")
	db.Model(&Event{}).Where("1 = 1").Update("created_at", time.Now().Add(-90*time.Second))
	if v := OldestPendingSeconds(db); v < 89 || v > 120 {
		t.Fatalf("age=%v", v)
	}
	db.Model(&Event{}).Where("1 = 1").Update("status", StatusDone)
	if v := OldestPendingSeconds(db); v != 0 {
		t.Fatalf("after publish: %v", v)
	}
}

func TestPollFromEnv(t *testing.T) {
	t.Setenv("OUTBOX_POLL_INTERVAL", "")
	t.Setenv("OUTBOX_BATCH_SIZE", "")
	if i, n := PollFromEnv(); i != DefaultPollInterval || n != DefaultBatchSize {
		t.Fatalf("defaults: %v %v", i, n)
	}
	t.Setenv("OUTBOX_POLL_INTERVAL", "50ms")
	t.Setenv("OUTBOX_BATCH_SIZE", "250")
	if i, n := PollFromEnv(); i != 50*time.Millisecond || n != 250 {
		t.Fatalf("env: %v %v", i, n)
	}
	t.Setenv("OUTBOX_POLL_INTERVAL", "garbage")
	t.Setenv("OUTBOX_BATCH_SIZE", "-3")
	if i, n := PollFromEnv(); i != DefaultPollInterval || n != DefaultBatchSize {
		t.Fatalf("bad env must fall back: %v %v", i, n)
	}
}
