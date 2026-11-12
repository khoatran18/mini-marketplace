package store

import (
	"context"
	"testing"
	"time"
)

func newIn(inc *int64, hash string) CreateInput {
	return CreateInput{IncidentID: inc, ActionID: "restart_service", Params: map[string]any{"service": "order-service"}, Risk: "low", Hash: hash,
		CreatedBy: "admin:99", TTL: time.Hour, MaxPending: 3, Now: time.Now()}
}

func openIncident(t *testing.T, s *Store) int64 {
	t.Helper()
	s.Observe(context.Background(), []Alert{alert("ServiceDown", "order-service")}, time.Now(), time.Minute)
	l, _ := s.List(context.Background(), "", 1)
	return l[0].ID
}

func TestProposalLifecycleAccept(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	inc := openIncident(t, s)
	p, err := s.CreateProposal(ctx, newIn(&inc, "h1"))
	if err != nil || p.State != PPending {
		t.Fatalf("%v %+v", err, p)
	}
	if _, err := s.Review(ctx, p.ID, "admin:1", "r1", time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Decide(ctx, p.ID, "admin:1", DecAccept, "ok", "r2", nil, time.Now())
	if err != nil || got.State != PAccepted || got.DecidedBy == nil || *got.DecidedBy != "admin:1" {
		t.Fatalf("%v %+v", err, got)
	}
	// không quyết hai lần
	if _, err := s.Decide(ctx, p.ID, "admin:1", DecReject, "x", "", nil, time.Now()); err != ErrBadState {
		t.Fatalf("lần hai phải ErrBadState, được %v", err)
	}
	trail, _ := s.AuditList(ctx, "proposal", "1", 10)
	var ev []string
	for _, e := range trail {
		ev = append(ev, e.Event)
	}
	if len(ev) != 3 || ev[0] != "proposal_accept" || ev[1] != "proposal_reviewed" || ev[2] != "proposal_created" {
		t.Fatalf("nhật ký: %v", ev)
	}
}

func TestCreatorCannotDecideOwnProposal(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p, _ := s.CreateProposal(ctx, newIn(nil, "h"))
	if _, err := s.Decide(ctx, p.ID, "admin:99", DecAccept, "tự duyệt", "", nil, time.Now()); err != ErrSelfApproval {
		t.Fatalf("phải ErrSelfApproval, được %v", err)
	}
	if got, _ := s.GetProposal(ctx, p.ID); got.State != PPending {
		t.Fatalf("trạng thái đổi dù bị từ chối: %s", got.State)
	}
}

func TestDuplicateAndCap(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	inc := openIncident(t, s)
	if _, err := s.CreateProposal(ctx, newIn(&inc, "same")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProposal(ctx, newIn(&inc, "same")); err != ErrDuplicate {
		t.Fatalf("trùng nội dung phải ErrDuplicate, được %v", err)
	}
	for _, h := range []string{"a", "b"} {
		if _, err := s.CreateProposal(ctx, newIn(&inc, h)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateProposal(ctx, newIn(&inc, "c")); err != ErrTooMany {
		t.Fatalf("quá giới hạn phải ErrTooMany, được %v", err)
	}
	bad := int64(424242)
	if _, err := s.CreateProposal(ctx, newIn(&bad, "z")); err != ErrNotFound {
		t.Fatalf("sự cố lạ phải ErrNotFound, được %v", err)
	}
}

func TestExpiry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in := newIn(nil, "h")
	in.TTL = time.Minute
	p, _ := s.CreateProposal(ctx, in)
	// quyết định sau hạn: EXPIRED + ErrExpired, và trạng thái được ghi
	if _, err := s.Decide(ctx, p.ID, "admin:1", DecAccept, "", "", nil, time.Now().Add(2*time.Minute)); err != ErrExpired {
		t.Fatalf("phải ErrExpired, được %v", err)
	}
	if got, _ := s.GetProposal(ctx, p.ID); got.State != PExpired {
		t.Fatalf("state %s", got.State)
	}
	// ExpireDue quét hàng loạt
	in2 := newIn(nil, "h2")
	in2.TTL = time.Minute
	s.CreateProposal(ctx, in2)
	if n, err := s.ExpireDue(ctx, time.Now().Add(5*time.Minute)); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if n, _ := s.ExpireDue(ctx, time.Now().Add(5*time.Minute)); n != 0 {
		t.Fatalf("lần hai phải 0, được %d", n)
	}
}

func TestNeedsReviewThenSupersede(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p, _ := s.CreateProposal(ctx, newIn(nil, "h1"))
	// chưa NEEDS_REVIEW thì không thay thế được
	in := newIn(nil, "h2")
	in.SupersedesID = &p.ID
	if _, err := s.CreateProposal(ctx, in); err != ErrBadState {
		t.Fatalf("phải ErrBadState, được %v", err)
	}
	if _, err := s.Decide(ctx, p.ID, "admin:1", DecNeedsReview, "thiếu log", "", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	n, err := s.CreateProposal(ctx, in)
	if err != nil || n.SupersedesID == nil || *n.SupersedesID != p.ID {
		t.Fatalf("%v %+v", err, n)
	}
}

func TestAlternativeRecordsChosenAction(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p, _ := s.CreateProposal(ctx, newIn(nil, "h1"))
	alt := &Alternative{ActionID: "scale_service", Params: map[string]any{"service": "order-service", "replicas": 2}, Risk: "low", Hash: "hh"}
	got, err := s.Decide(ctx, p.ID, "admin:1", DecAlternative, "scale đủ rồi", "", alt, time.Now())
	if err != nil || got.State != PAlternative || got.ChosenActionID == nil || *got.ChosenActionID != "scale_service" || *got.ChosenHash != "hh" {
		t.Fatalf("%v %+v", err, got)
	}
	// nội dung gốc không đổi
	if got.ActionID != "restart_service" || got.ContentHash != "h1" {
		t.Fatalf("nội dung gốc bị đổi: %+v", got)
	}
	// decision=alternative mà thiếu alt (hoặc ngược lại) bị chặn
	p2, _ := s.CreateProposal(ctx, newIn(nil, "h2"))
	if _, err := s.Decide(ctx, p2.ID, "admin:1", DecAlternative, "x", "", nil, time.Now()); err == nil {
		t.Fatal("alternative thiếu alt phải lỗi")
	}
	if _, err := s.Decide(ctx, p2.ID, "admin:1", DecAccept, "x", "", alt, time.Now()); err == nil {
		t.Fatal("accept kèm alt phải lỗi")
	}
}

func TestStaleExecutionsFail(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO ops_proposals (action_id, risk, state, content_hash, created_by, expires_at, exec_state, exec_by, exec_started_at)
		VALUES ('requeue_outbox','high','ACCEPTED','h','a', now()+interval '1 day','EXECUTING','b',$1)`, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	n, err := s.FailStaleExecutions(ctx, 10*time.Minute, now)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var st string
	_ = s.DB.QueryRowContext(ctx, `SELECT exec_state FROM ops_proposals`).Scan(&st)
	if st != "FAILED" {
		t.Fatalf("state=%s", st)
	}
	// exec_state chỉ hợp lệ với đề xuất đã duyệt
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO ops_proposals (action_id, risk, state, content_hash, created_by, expires_at, exec_state) VALUES ('x','low','PENDING','h2','a', now(),'EXECUTED')`); err == nil {
		t.Fatal("PENDING không được có exec_state")
	}
}
