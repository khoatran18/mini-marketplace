package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Trạng thái đề xuất.
const (
	PPending     = "PENDING"
	PAccepted    = "ACCEPTED"
	PRejected    = "REJECTED"
	PNeedsReview = "NEEDS_REVIEW"
	PAlternative = "ALTERNATIVE"
	PExpired     = "EXPIRED"
)

// Quyết định của người duyệt.
const (
	DecAccept      = "accept"
	DecReject      = "reject"
	DecNeedsReview = "needs_review"
	DecAlternative = "alternative"
)

// Lỗi của luồng đề xuất.
var (
	ErrExpired      = errors.New("proposal expired")
	ErrSelfApproval = errors.New("creator cannot decide own proposal")
	ErrTooMany      = errors.New("too many pending proposals for this incident")
	ErrDuplicate    = errors.New("an identical proposal is already pending")
)

// Proposal là một dòng ops_proposals.
type Proposal struct {
	ID             int64           `json:"id"`
	IncidentID     *int64          `json:"incident_id,omitempty"`
	ActionID       string          `json:"action_id"`
	Params         json.RawMessage `json:"params"`
	Evidence       json.RawMessage `json:"evidence"`
	Rationale      string          `json:"rationale"`
	Risk           string          `json:"risk"`
	State          string          `json:"state"`
	ContentHash    string          `json:"content_hash"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	ExpiresAt      time.Time       `json:"expires_at"`
	SupersedesID   *int64          `json:"supersedes_id,omitempty"`
	DecidedBy      *string         `json:"decided_by,omitempty"`
	DecidedAt      *time.Time      `json:"decided_at,omitempty"`
	DecisionReason string          `json:"decision_reason,omitempty"`
	ChosenActionID *string         `json:"chosen_action_id,omitempty"`
	ChosenParams   json.RawMessage `json:"chosen_params,omitempty"`
	ChosenRisk     *string         `json:"chosen_risk,omitempty"`
	ChosenHash     *string         `json:"chosen_hash,omitempty"`
	ExecState      *string         `json:"exec_state,omitempty"`
	ExecBy         *string         `json:"exec_by,omitempty"`
	ExecStartedAt  *time.Time      `json:"exec_started_at,omitempty"`
	ExecFinishedAt *time.Time      `json:"exec_finished_at,omitempty"`
	ExecResult     json.RawMessage `json:"exec_result,omitempty"`
}

// Effective trả hành động thật sự được duyệt: hành động người duyệt chọn (ALTERNATIVE) hoặc hành động gốc.
func (p Proposal) Effective() (actionID string, params map[string]any, risk string) {
	raw, actionID, risk := p.Params, p.ActionID, p.Risk
	if p.ChosenActionID != nil {
		raw, actionID = p.ChosenParams, *p.ChosenActionID
		if p.ChosenRisk != nil {
			risk = *p.ChosenRisk
		}
	}
	_ = json.Unmarshal(raw, &params)
	return actionID, params, risk
}

// CreateInput là dữ liệu đã được lớp API kiểm (hành động nằm trong sổ, tham số chuẩn hoá, rủi ro lấy từ sổ).
type CreateInput struct {
	IncidentID   *int64
	ActionID     string
	Params       map[string]any
	Evidence     []string
	Rationale    string
	Risk         string
	Hash         string
	CreatedBy    string
	RequestID    string
	SupersedesID *int64
	TTL          time.Duration
	MaxPending   int // tối đa đề xuất PENDING cho một sự cố (0 = không giới hạn)
	Now          time.Time
}

// Alternative là hành động thay thế do người duyệt chọn (đã được kiểm với sổ hành động).
type Alternative struct {
	ActionID string
	Params   map[string]any
	Risk     string
	Hash     string
}

const pcols = `id, incident_id, action_id, params, evidence, rationale, risk, state, content_hash, created_by, created_at, expires_at,
 supersedes_id, decided_by, decided_at, decision_reason, chosen_action_id, chosen_params, chosen_risk, chosen_hash,
 exec_state, exec_by, exec_started_at, exec_finished_at, exec_result`

func scanP(r interface{ Scan(...any) error }) (Proposal, error) {
	var p Proposal
	var inc, sup sql.NullInt64
	var decBy, chAct, chRisk, chHash, exState, exBy sql.NullString
	var decAt, exStart, exEnd sql.NullTime
	var chParams, exResult []byte
	err := r.Scan(&p.ID, &inc, &p.ActionID, &p.Params, &p.Evidence, &p.Rationale, &p.Risk, &p.State, &p.ContentHash, &p.CreatedBy, &p.CreatedAt, &p.ExpiresAt,
		&sup, &decBy, &decAt, &p.DecisionReason, &chAct, &chParams, &chRisk, &chHash,
		&exState, &exBy, &exStart, &exEnd, &exResult)
	if err != nil {
		return p, err
	}
	if inc.Valid {
		p.IncidentID = &inc.Int64
	}
	if sup.Valid {
		p.SupersedesID = &sup.Int64
	}
	if decBy.Valid {
		p.DecidedBy = &decBy.String
	}
	if decAt.Valid {
		p.DecidedAt = &decAt.Time
	}
	if chAct.Valid {
		p.ChosenActionID, p.ChosenParams = &chAct.String, chParams
		p.ChosenRisk, p.ChosenHash = &chRisk.String, &chHash.String
	}
	if exState.Valid {
		p.ExecState, p.ExecResult = &exState.String, exResult
	}
	if exBy.Valid {
		p.ExecBy = &exBy.String
	}
	if exStart.Valid {
		p.ExecStartedAt = &exStart.Time
	}
	if exEnd.Valid {
		p.ExecFinishedAt = &exEnd.Time
	}
	return p, nil
}

func isUnique(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key"))
}

// CreateProposal tạo đề xuất PENDING và ghi nhật ký.
func (s *Store) CreateProposal(ctx context.Context, in CreateInput) (Proposal, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Proposal{}, err
	}
	defer tx.Rollback()
	if in.IncidentID != nil {
		var one int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM ops_incidents WHERE id=$1`, *in.IncidentID).Scan(&one); errors.Is(err, sql.ErrNoRows) {
			return Proposal{}, ErrNotFound
		} else if err != nil {
			return Proposal{}, err
		}
		if in.MaxPending > 0 {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM ops_proposals WHERE incident_id=$1 AND state='PENDING'`, *in.IncidentID).Scan(&n); err != nil {
				return Proposal{}, err
			}
			if n >= in.MaxPending {
				return Proposal{}, ErrTooMany
			}
		}
	}
	if in.SupersedesID != nil {
		var st string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM ops_proposals WHERE id=$1`, *in.SupersedesID).Scan(&st); errors.Is(err, sql.ErrNoRows) {
			return Proposal{}, ErrNotFound
		} else if err != nil {
			return Proposal{}, err
		}
		if st != PNeedsReview {
			return Proposal{}, ErrBadState
		}
	}
	params, _ := json.Marshal(in.Params)
	if in.Evidence == nil {
		in.Evidence = []string{}
	}
	ev, _ := json.Marshal(in.Evidence)
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO ops_proposals (incident_id, action_id, params, evidence, rationale, risk, content_hash, created_by, created_at, expires_at, supersedes_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		in.IncidentID, in.ActionID, params, ev, in.Rationale, in.Risk, in.Hash, in.CreatedBy, in.Now, in.Now.Add(in.TTL), in.SupersedesID).Scan(&id)
	if isUnique(err) {
		return Proposal{}, ErrDuplicate
	} else if err != nil {
		return Proposal{}, err
	}
	if err := audit(ctx, tx, in.CreatedBy, "proposal_created", "proposal", fmt.Sprint(id), in.RequestID,
		map[string]any{"action_id": in.ActionID, "risk": in.Risk, "hash": in.Hash, "incident_id": in.IncidentID, "supersedes_id": in.SupersedesID}); err != nil {
		return Proposal{}, err
	}
	p, err := scanP(tx.QueryRowContext(ctx, `SELECT `+pcols+` FROM ops_proposals WHERE id=$1`, id))
	if err != nil {
		return Proposal{}, err
	}
	return p, tx.Commit()
}

// GetProposal trả một đề xuất.
func (s *Store) GetProposal(ctx context.Context, id int64) (Proposal, error) {
	p, err := scanP(s.DB.QueryRowContext(ctx, `SELECT `+pcols+` FROM ops_proposals WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// ListProposals trả đề xuất mới nhất trước; lọc theo state và/hoặc sự cố.
func (s *Store) ListProposals(ctx context.Context, state string, incidentID int64, limit int) ([]Proposal, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q, args := `SELECT `+pcols+` FROM ops_proposals WHERE true`, []any{}
	if state != "" {
		args = append(args, state)
		q += fmt.Sprintf(" AND state=$%d", len(args))
	}
	if incidentID > 0 {
		args = append(args, incidentID)
		q += fmt.Sprintf(" AND incident_id=$%d", len(args))
	}
	q += fmt.Sprintf(" ORDER BY id DESC LIMIT %d", limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Proposal{}
	for rows.Next() {
		p, err := scanP(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Review kiểm đề xuất còn PENDING và chưa hết hạn, ghi nhật ký "đã xem", trả đề xuất để lớp API cấp mã duyệt.
func (s *Store) Review(ctx context.Context, id int64, actor, requestID string, now time.Time) (Proposal, error) {
	p, err := s.GetProposal(ctx, id)
	if err != nil {
		return p, err
	}
	if p.State != PPending {
		return p, ErrBadState
	}
	if !now.Before(p.ExpiresAt) {
		return p, ErrExpired
	}
	if err := s.Audit(ctx, actor, "proposal_reviewed", "proposal", fmt.Sprint(id), requestID, map[string]any{"hash": p.ContentHash}); err != nil {
		return p, err
	}
	return p, nil
}

// Decide áp dụng quyết định lên đề xuất PENDING (khoá dòng). Người tạo không được tự quyết. Hết hạn thì chuyển EXPIRED.
func (s *Store) Decide(ctx context.Context, id int64, actor, decision, reason, requestID string, alt *Alternative, now time.Time) (Proposal, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Proposal{}, err
	}
	defer tx.Rollback()
	p, err := scanP(tx.QueryRowContext(ctx, `SELECT `+pcols+` FROM ops_proposals WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	} else if err != nil {
		return p, err
	}
	if p.State != PPending {
		return p, ErrBadState
	}
	if !now.Before(p.ExpiresAt) {
		if _, err := tx.ExecContext(ctx, `UPDATE ops_proposals SET state='EXPIRED' WHERE id=$1`, id); err != nil {
			return p, err
		}
		if err := audit(ctx, tx, "system", "proposal_expired", "proposal", fmt.Sprint(id), requestID, map[string]any{}); err != nil {
			return p, err
		}
		if err := tx.Commit(); err != nil {
			return p, err
		}
		return p, ErrExpired
	}
	if p.CreatedBy == actor {
		return p, ErrSelfApproval
	}
	newState := map[string]string{DecAccept: PAccepted, DecReject: PRejected, DecNeedsReview: PNeedsReview, DecAlternative: PAlternative}[decision]
	if newState == "" || (decision == DecAlternative) != (alt != nil) {
		return p, fmt.Errorf("invalid decision")
	}
	if alt != nil {
		ap, _ := json.Marshal(alt.Params)
		_, err = tx.ExecContext(ctx, `UPDATE ops_proposals SET state=$2, decided_by=$3, decided_at=$4, decision_reason=$5, chosen_action_id=$6, chosen_params=$7, chosen_risk=$8, chosen_hash=$9 WHERE id=$1`,
			id, newState, actor, now, reason, alt.ActionID, ap, alt.Risk, alt.Hash)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE ops_proposals SET state=$2, decided_by=$3, decided_at=$4, decision_reason=$5 WHERE id=$1`, id, newState, actor, now, reason)
	}
	if err != nil {
		return p, err
	}
	details := map[string]any{"decision": decision, "reason": reason, "risk": p.Risk, "hash": p.ContentHash}
	if alt != nil {
		details["alternative"] = map[string]any{"action_id": alt.ActionID, "params": alt.Params, "risk": alt.Risk, "hash": alt.Hash}
	}
	if err := audit(ctx, tx, actor, "proposal_"+decision, "proposal", fmt.Sprint(id), requestID, details); err != nil {
		return p, err
	}
	p, err = scanP(tx.QueryRowContext(ctx, `SELECT `+pcols+` FROM ops_proposals WHERE id=$1`, id))
	if err != nil {
		return p, err
	}
	return p, tx.Commit()
}

// ExpireDue chuyển các đề xuất PENDING quá hạn sang EXPIRED (mỗi cái một dòng nhật ký). Trả số đã chuyển.
func (s *Store) ExpireDue(ctx context.Context, now time.Time) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `UPDATE ops_proposals SET state='EXPIRED' WHERE state='PENDING' AND expires_at <= $1 RETURNING id`, now)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := audit(ctx, tx, "system", "proposal_expired", "proposal", fmt.Sprint(id), "", map[string]any{}); err != nil {
			return 0, err
		}
	}
	return len(ids), tx.Commit()
}

// CountProposalsByState phục vụ metric.
func (s *Store) CountProposalsByState(ctx context.Context) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT state, count(*) FROM ops_proposals GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		m[st] = n
	}
	return m, rows.Err()
}
