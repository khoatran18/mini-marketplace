package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Trạng thái thực thi (cột exec_state).
const (
	ExecRunning = "EXECUTING"
	ExecDone    = "EXECUTED"
	ExecFailed  = "FAILED"
)

// Lỗi của luồng thực thi.
var (
	ErrAlreadyExecuted = errors.New("proposal already executed or executing")
	ErrRateLimited     = errors.New("execution rate limit reached")
	ErrCooldown        = errors.New("same action on the same service ran recently")
	ErrApprovalStale   = errors.New("approval is too old to execute")
)

// ExecLimits là các chặn khi thực thi: tối đa mỗi giờ, nghỉ giữa hai lần cùng (hành động, service), và hạn dùng của quyết định duyệt.
type ExecLimits struct {
	MaxPerHour int
	Cooldown   time.Duration
	Window     time.Duration
}

// ClaimExecution giành quyền thực thi (một lần duy nhất): đề xuất phải ACCEPTED/ALTERNATIVE, chưa thực thi, quyết định chưa quá hạn,
// người chạy không phải người tạo, chưa vượt hạn mức, không trùng cooldown. Đặt exec_state=EXECUTING trong cùng transaction.
// Khoá advisory làm các lần giành quyền xếp hàng nên hạn mức không bị vượt khi chạy song song.
func (s *Store) ClaimExecution(ctx context.Context, id int64, actor, requestID string, lim ExecLimits, now time.Time) (Proposal, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Proposal{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7311001)`); err != nil {
		return Proposal{}, err
	}
	p, err := scanP(tx.QueryRowContext(ctx, `SELECT `+pcols+` FROM ops_proposals WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	} else if err != nil {
		return p, err
	}
	if p.State != PAccepted && p.State != PAlternative {
		return p, ErrBadState
	}
	if p.ExecState != nil {
		return p, ErrAlreadyExecuted
	}
	if p.CreatedBy == actor {
		return p, ErrSelfApproval
	}
	if lim.Window > 0 && (p.DecidedAt == nil || now.After(p.DecidedAt.Add(lim.Window))) {
		return p, ErrApprovalStale
	}
	if lim.MaxPerHour > 0 {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM ops_proposals WHERE exec_started_at > $1`, now.Add(-time.Hour)).Scan(&n); err != nil {
			return p, err
		}
		if n >= lim.MaxPerHour {
			return p, ErrRateLimited
		}
	}
	actionID, params, risk := p.Effective()
	svc, _ := params["service"].(string)
	if lim.Cooldown > 0 {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM ops_proposals WHERE id <> $1 AND exec_started_at > $2
			AND COALESCE(chosen_action_id, action_id) = $3 AND COALESCE(COALESCE(chosen_params, params)->>'service','') = $4`,
			id, now.Add(-lim.Cooldown), actionID, svc).Scan(&n); err != nil {
			return p, err
		}
		if n > 0 {
			return p, ErrCooldown
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ops_proposals SET exec_state='EXECUTING', exec_by=$2, exec_started_at=$3 WHERE id=$1`, id, actor, now); err != nil {
		return p, err
	}
	if err := audit(ctx, tx, actor, "proposal_execution_started", "proposal", fmt.Sprint(id), requestID,
		map[string]any{"action_id": actionID, "params": params, "risk": risk, "state": p.State}); err != nil {
		return p, err
	}
	p, err = scanP(tx.QueryRowContext(ctx, `SELECT `+pcols+` FROM ops_proposals WHERE id=$1`, id))
	if err != nil {
		return p, err
	}
	return p, tx.Commit()
}

// FinishExecution ghi kết quả của lần thực thi đang EXECUTING.
func (s *Store) FinishExecution(ctx context.Context, id int64, ok bool, result any, actor, requestID string, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st := ExecFailed
	if ok {
		st = ExecDone
	}
	b, _ := json.Marshal(result)
	res, err := tx.ExecContext(ctx, `UPDATE ops_proposals SET exec_state=$2, exec_finished_at=$3, exec_result=$4 WHERE id=$1 AND exec_state='EXECUTING'`, id, st, now, b)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrBadState
	}
	if err := audit(ctx, tx, actor, "proposal_execution_"+map[bool]string{true: "succeeded", false: "failed"}[ok], "proposal", fmt.Sprint(id), requestID, result); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordManual ghi kết quả khi người tự chạy lệnh (hành động `command`/`manual`). Người ghi không được là người tạo đề xuất.
func (s *Store) RecordManual(ctx context.Context, id int64, actor, requestID string, ok bool, note string, now time.Time) (Proposal, error) {
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
	if p.State != PAccepted && p.State != PAlternative {
		return p, ErrBadState
	}
	if p.ExecState != nil {
		return p, ErrAlreadyExecuted
	}
	if p.CreatedBy == actor {
		return p, ErrSelfApproval
	}
	st := ExecFailed
	if ok {
		st = ExecDone
	}
	result, _ := json.Marshal(map[string]any{"manual": true, "note": note})
	if _, err := tx.ExecContext(ctx, `UPDATE ops_proposals SET exec_state=$2, exec_by=$3, exec_finished_at=$4, exec_result=$5 WHERE id=$1`, id, st, actor, now, result); err != nil {
		return p, err
	}
	actionID, params, _ := p.Effective()
	if err := audit(ctx, tx, actor, "proposal_manual_"+map[bool]string{true: "succeeded", false: "failed"}[ok], "proposal", fmt.Sprint(id), requestID,
		map[string]any{"action_id": actionID, "params": params, "note": note}); err != nil {
		return p, err
	}
	p, err = scanP(tx.QueryRowContext(ctx, `SELECT `+pcols+` FROM ops_proposals WHERE id=$1`, id))
	if err != nil {
		return p, err
	}
	return p, tx.Commit()
}

// FailStaleExecutions đánh dấu FAILED các lần EXECUTING quá `olderThan` (tiến trình chết giữa chừng): không biết hành động đã chạy chưa
// nên KHÔNG tự chạy lại; người kiểm tra tay rồi tạo đề xuất mới nếu cần.
func (s *Store) FailStaleExecutions(ctx context.Context, olderThan time.Duration, now time.Time) (int, error) {
	rows, err := s.DB.QueryContext(ctx, `UPDATE ops_proposals SET exec_state='FAILED', exec_finished_at=$2, exec_result='{"error":"interrupted: tiến trình dừng giữa chừng, kiểm tra tay"}'
		WHERE exec_state='EXECUTING' AND exec_started_at < $1 RETURNING id`, now.Add(-olderThan), now)
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
		if err := s.Audit(ctx, "system", "proposal_execution_failed", "proposal", fmt.Sprint(id), "", map[string]any{"error": "interrupted"}); err != nil {
			return len(ids), err
		}
	}
	return len(ids), rows.Err()
}
