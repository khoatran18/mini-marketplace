// Package store lưu sự cố (ops_incidents) và nhật ký bất biến (ops_audit_log).
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Trạng thái sự cố.
const (
	StateOpen          = "open"
	StateInvestigating = "investigating"
	StateResolved      = "resolved"
)

// ErrNotFound khi không có sự cố; ErrBadState khi chuyển trạng thái không hợp lệ.
var (
	ErrNotFound = errors.New("not found")
	ErrBadState = errors.New("invalid state transition")
)

// volatile là nhãn thay đổi theo thời gian chạy, không dùng để nhận diện cùng một sự cố.
var volatile = map[string]bool{"instance": true, "container_label_com_docker_swarm_task_id": true, "id": true}

// Alert là một cảnh báo đang bắn (đã bỏ phần chi tiết của Prometheus).
type Alert struct {
	Name        string
	Labels      map[string]string
	Annotations map[string]string
}

// DedupeKey ổn định cho cùng cảnh báo + cùng thành phần (bỏ nhãn thay đổi như instance).
func DedupeKey(a Alert) string {
	keys := make([]string, 0, len(a.Labels))
	for k := range a.Labels {
		if k == "alertname" || k == "severity" || volatile[k] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(a.Name)
	for _, k := range keys {
		b.WriteString("|" + k + "=" + a.Labels[k])
	}
	if len(keys) == 0 {
		return a.Name
	}
	sum := sha256.Sum256([]byte(b.String()))
	return a.Name + ":" + hex.EncodeToString(sum[:6])
}

// Component lấy tên thành phần dễ đọc từ nhãn.
func Component(l map[string]string) string {
	for _, k := range []string{"swarm_service", "service", "container_label_com_docker_swarm_service_name", "table", "topic", "job"} {
		if v := l[k]; v != "" {
			return v
		}
	}
	return ""
}

// Incident là một dòng ops_incidents.
type Incident struct {
	ID         int64             `json:"id"`
	DedupeKey  string            `json:"dedupe_key"`
	AlertName  string            `json:"alert_name"`
	Severity   string            `json:"severity"`
	Component  string            `json:"component"`
	Labels     map[string]string `json:"labels"`
	Summary    string            `json:"summary"`
	Runbook    string            `json:"runbook"`
	Action     string            `json:"action"`
	State      string            `json:"state"`
	FireCount  int               `json:"fire_count"`
	FirstSeen  time.Time         `json:"first_seen"`
	LastSeen   time.Time         `json:"last_seen"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
}

// AuditEntry là một dòng ops_audit_log.
type AuditEntry struct {
	ID          int64           `json:"id"`
	TS          time.Time       `json:"ts"`
	Actor       string          `json:"actor"`
	Event       string          `json:"event"`
	SubjectType string          `json:"subject_type"`
	SubjectID   string          `json:"subject_id"`
	Details     json.RawMessage `json:"details"`
	RequestID   string          `json:"request_id,omitempty"`
}

// Store bọc *sql.DB.
type Store struct{ DB *sql.DB }

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// Audit ghi một dòng nhật ký (dùng bên trong transaction hoặc trực tiếp).
func audit(ctx context.Context, x execer, actor, event, subjectType, subjectID, requestID string, details any) error {
	d, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = x.ExecContext(ctx, `INSERT INTO ops_audit_log (actor, event, subject_type, subject_id, details, request_id) VALUES ($1,$2,$3,$4,$5,$6)`,
		actor, event, subjectType, subjectID, d, requestID)
	return err
}

// Audit ghi nhật ký độc lập.
func (s *Store) Audit(ctx context.Context, actor, event, subjectType, subjectID, requestID string, details any) error {
	return audit(ctx, s.DB, actor, event, subjectType, subjectID, requestID, details)
}

// Observe áp dụng danh sách cảnh báo đang bắn: mở sự cố mới hoặc làm mới sự cố đang có; sự cố đã biến mất quá `grace` thì resolved.
// Trả về số sự cố mới mở và số đã resolved.
func (s *Store) Observe(ctx context.Context, firing []Alert, now time.Time, grace time.Duration) (opened, resolved int, err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	seen := map[string]bool{}
	for _, a := range firing {
		key := DedupeKey(a)
		if seen[key] { // nhiều instance cùng cảnh báo: một sự cố
			continue
		}
		seen[key] = true
		labels, _ := json.Marshal(a.Labels)
		res, err := tx.ExecContext(ctx, `UPDATE ops_incidents SET last_seen=$2, fire_count=fire_count+1 WHERE dedupe_key=$1 AND state<>'resolved'`, key, now)
		if err != nil {
			return 0, 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			continue
		}
		act := a.Annotations["action"]
		if act == "" {
			act = "observe"
		}
		var id int64
		err = tx.QueryRowContext(ctx, `INSERT INTO ops_incidents (dedupe_key, alert_name, severity, component, labels, summary, runbook, action, first_seen, last_seen)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9) RETURNING id`,
			key, a.Name, a.Labels["severity"], Component(a.Labels), labels, a.Annotations["summary"], a.Annotations["runbook"], act, now).Scan(&id)
		if err != nil {
			return 0, 0, err
		}
		opened++
		if err := audit(ctx, tx, "poller", "incident_opened", "incident", fmt.Sprint(id), "", map[string]any{"alert": a.Name, "dedupe_key": key, "action": act}); err != nil {
			return 0, 0, err
		}
	}

	rows, err := tx.QueryContext(ctx, `SELECT id, dedupe_key FROM ops_incidents WHERE state<>'resolved' AND last_seen < $1`, now.Add(-grace))
	if err != nil {
		return 0, 0, err
	}
	type gone struct {
		id  int64
		key string
	}
	var gs []gone
	for rows.Next() {
		var g gone
		if err := rows.Scan(&g.id, &g.key); err != nil {
			rows.Close()
			return 0, 0, err
		}
		if !seen[g.key] {
			gs = append(gs, g)
		}
	}
	rows.Close()
	for _, g := range gs {
		if _, err := tx.ExecContext(ctx, `UPDATE ops_incidents SET state='resolved', resolved_at=$2 WHERE id=$1`, g.id, now); err != nil {
			return 0, 0, err
		}
		resolved++
		if err := audit(ctx, tx, "poller", "incident_resolved", "incident", fmt.Sprint(g.id), "", map[string]any{"dedupe_key": g.key}); err != nil {
			return 0, 0, err
		}
	}
	return opened, resolved, tx.Commit()
}

const cols = `id, dedupe_key, alert_name, severity, component, labels, summary, runbook, action, state, fire_count, first_seen, last_seen, resolved_at`

func scan(r interface{ Scan(...any) error }) (Incident, error) {
	var i Incident
	var labels []byte
	var res sql.NullTime
	if err := r.Scan(&i.ID, &i.DedupeKey, &i.AlertName, &i.Severity, &i.Component, &labels, &i.Summary, &i.Runbook, &i.Action, &i.State, &i.FireCount, &i.FirstSeen, &i.LastSeen, &res); err != nil {
		return i, err
	}
	_ = json.Unmarshal(labels, &i.Labels)
	if res.Valid {
		t := res.Time
		i.ResolvedAt = &t
	}
	return i, nil
}

// List trả sự cố mới nhất trước; state rỗng = tất cả; limit tối đa 200.
func (s *Store) List(ctx context.Context, state string, limit int) ([]Incident, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT ` + cols + ` FROM ops_incidents`
	args := []any{}
	if state != "" {
		q += ` WHERE state=$1`
		args = append(args, state)
	}
	q += fmt.Sprintf(` ORDER BY last_seen DESC, id DESC LIMIT %d`, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Incident{}
	for rows.Next() {
		i, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Get trả một sự cố.
func (s *Store) Get(ctx context.Context, id int64) (Incident, error) {
	i, err := scan(s.DB.QueryRowContext(ctx, `SELECT `+cols+` FROM ops_incidents WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return i, ErrNotFound
	}
	return i, err
}

// Investigate đánh dấu đang điều tra (open -> investigating) và ghi nhật ký.
func (s *Store) Investigate(ctx context.Context, id int64, actor, requestID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE ops_incidents SET state='investigating' WHERE id=$1 AND state='open'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var st string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM ops_incidents WHERE id=$1`, id).Scan(&st); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return ErrBadState
	}
	if err := audit(ctx, tx, actor, "incident_investigating", "incident", fmt.Sprint(id), requestID, map[string]any{}); err != nil {
		return err
	}
	return tx.Commit()
}

// AuditList trả nhật ký mới nhất trước, lọc theo đối tượng nếu có.
func (s *Store) AuditList(ctx context.Context, subjectType, subjectID string, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, ts, actor, event, subject_type, subject_id, details, request_id FROM ops_audit_log`
	args := []any{}
	if subjectType != "" {
		q += ` WHERE subject_type=$1 AND subject_id=$2`
		args = append(args, subjectType, subjectID)
	}
	q += fmt.Sprintf(` ORDER BY id DESC LIMIT %d`, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var d []byte
		if err := rows.Scan(&e.ID, &e.TS, &e.Actor, &e.Event, &e.SubjectType, &e.SubjectID, &d, &e.RequestID); err != nil {
			return nil, err
		}
		e.Details = d
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountByState phục vụ metric.
func (s *Store) CountByState(ctx context.Context) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT state, count(*) FROM ops_incidents WHERE state<>'resolved' GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		m[s] = n
	}
	return m, rows.Err()
}

// MaxAuditID trả id nhật ký lớn nhất (0 nếu rỗng); notifier bắt đầu từ đây để không gửi lại lịch sử.
func (s *Store) MaxAuditID(ctx context.Context) (int64, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM ops_audit_log`).Scan(&id)
	return id, err
}

// AuditSince trả các dòng nhật ký có id > after thuộc `events`, tăng dần theo id.
func (s *Store) AuditSince(ctx context.Context, after int64, events []string, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, ts, actor, event, subject_type, subject_id, details, request_id FROM ops_audit_log
		WHERE id > $1 AND event = ANY($2) ORDER BY id ASC LIMIT $3`, after, events, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var d []byte
		if err := rows.Scan(&e.ID, &e.TS, &e.Actor, &e.Event, &e.SubjectType, &e.SubjectID, &d, &e.RequestID); err != nil {
			return nil, err
		}
		e.Details = d
		out = append(out, e)
	}
	return out, rows.Err()
}
