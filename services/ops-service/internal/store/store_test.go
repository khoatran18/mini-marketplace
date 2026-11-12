package store

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"ops-service/internal/migrate"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("ops_test_%d", rand.Int63())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn+" dbname="+name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"); admin.Close() })
	if err := migrate.Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return &Store{DB: db}
}

func alert(name, svc string) Alert {
	return Alert{Name: name, Labels: map[string]string{"alertname": name, "swarm_service": svc, "severity": "critical", "instance": "10.0.0.5:8081"},
		Annotations: map[string]string{"summary": "s", "runbook": "§1", "action": "low"}}
}

func TestDedupeKeyIgnoresInstanceAndSeverity(t *testing.T) {
	a := alert("ServiceDown", "payment-service")
	b := alert("ServiceDown", "payment-service")
	b.Labels["instance"] = "10.0.0.9:8081"
	b.Labels["severity"] = "warning"
	if DedupeKey(a) != DedupeKey(b) {
		t.Fatal("cùng cảnh báo cùng thành phần phải cùng khoá")
	}
	if DedupeKey(a) == DedupeKey(alert("ServiceDown", "order-service")) {
		t.Fatal("thành phần khác phải khoá khác")
	}
	if DedupeKey(a) == DedupeKey(alert("ServiceNotReady", "payment-service")) {
		t.Fatal("cảnh báo khác phải khoá khác")
	}
}

func TestObserveOpensOnceAndRefreshes(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	for i := 0; i < 3; i++ {
		// hai instance cùng cảnh báo trong một lần đọc vẫn là một sự cố
		o, _, err := s.Observe(ctx, []Alert{alert("ServiceDown", "payment-service"), alert("ServiceDown", "payment-service")}, now.Add(time.Duration(i)*time.Second), time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if (i == 0 && o != 1) || (i > 0 && o != 0) {
			t.Fatalf("lần %d mở %d sự cố", i, o)
		}
	}
	list, _ := s.List(ctx, "", 10)
	if len(list) != 1 || list[0].FireCount != 3 || list[0].State != StateOpen || list[0].Component != "payment-service" || list[0].Action != "low" {
		t.Fatalf("danh sách sai: %+v", list)
	}
}

func TestObserveResolvesOnlyAfterGrace(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	t0 := time.Now()
	s.Observe(ctx, []Alert{alert("ServiceDown", "x")}, t0, time.Minute)
	// biến mất nhưng chưa quá grace: giữ nguyên
	if _, r, _ := s.Observe(ctx, nil, t0.Add(30*time.Second), time.Minute); r != 0 {
		t.Fatalf("resolve quá sớm: %d", r)
	}
	if _, r, _ := s.Observe(ctx, nil, t0.Add(2*time.Minute), time.Minute); r != 1 {
		t.Fatalf("phải resolve: %d", r)
	}
	// bắn lại sau khi resolved: sự cố MỚI
	o, _, _ := s.Observe(ctx, []Alert{alert("ServiceDown", "x")}, t0.Add(3*time.Minute), time.Minute)
	list, _ := s.List(ctx, "", 10)
	if o != 1 || len(list) != 2 {
		t.Fatalf("bắn lại phải mở sự cố mới: o=%d n=%d", o, len(list))
	}
	trail, _ := s.AuditList(ctx, "", "", 50)
	if len(trail) != 3 { // opened, resolved, opened
		t.Fatalf("audit %d dòng", len(trail))
	}
}

func TestInvestigateTransitions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	s.Observe(ctx, []Alert{alert("ServiceDown", "x")}, time.Now(), time.Minute)
	list, _ := s.List(ctx, "", 10)
	id := list[0].ID
	if err := s.Investigate(ctx, id, "agent", "req-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Investigate(ctx, id, "agent", ""); err != ErrBadState {
		t.Fatalf("lần hai phải ErrBadState, được %v", err)
	}
	if err := s.Investigate(ctx, 999999, "agent", ""); err != ErrNotFound {
		t.Fatalf("id lạ phải ErrNotFound, được %v", err)
	}
	trail, _ := s.AuditList(ctx, "incident", fmt.Sprint(id), 10)
	if len(trail) != 2 || trail[0].Event != "incident_investigating" || trail[0].Actor != "agent" || trail[0].RequestID != "req-1" {
		t.Fatalf("audit sai: %+v", trail)
	}
}

func TestAuditLogIsAppendOnly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.Audit(ctx, "admin:1", "test", "x", "1", "", map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE ops_audit_log SET actor='z'`, `DELETE FROM ops_audit_log`, `TRUNCATE ops_audit_log`} {
		if _, err := s.DB.ExecContext(ctx, q); err == nil {
			t.Errorf("%q phải bị từ chối", q)
		}
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM ops_audit_log`).Scan(&n)
	if n != 1 {
		t.Fatalf("dòng nhật ký bị mất: %d", n)
	}
}

func TestOnlyOneActiveIncidentPerKey(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ins := `INSERT INTO ops_incidents (dedupe_key, alert_name) VALUES ('k','A')`
	if _, err := s.DB.ExecContext(ctx, ins); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, ins); err == nil {
		t.Fatal("chỉ được một sự cố chưa resolved cho cùng khoá")
	}
}

func TestAuditSince(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, ev := range []string{"incident_opened", "query.run", "proposal_created"} {
		if err := s.Audit(ctx, "x", ev, "t", "1", "", map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	max, err := s.MaxAuditID(ctx)
	if err != nil || max < 3 {
		t.Fatalf("max=%d err=%v", max, err)
	}
	got, err := s.AuditSince(ctx, 0, []string{"incident_opened", "proposal_created"}, 10)
	if err != nil || len(got) != 2 || got[0].Event != "incident_opened" || got[0].ID >= got[1].ID {
		t.Fatalf("lọc sai: %+v %v", got, err)
	}
	if got, _ := s.AuditSince(ctx, max, []string{"incident_opened"}, 10); len(got) != 0 {
		t.Fatalf("sau max phải rỗng: %+v", got)
	}
}
