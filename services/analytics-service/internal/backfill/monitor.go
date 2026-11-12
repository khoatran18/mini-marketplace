package backfill

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"analytics-service/internal/ch"
)

// Monitor runs Reconcile on a schedule over the most recent days and keeps the last result, so data-health and
// Prometheus can show whether ClickHouse still agrees with Postgres (ADR-37). It needs a READ-ONLY Postgres
// connection; analytics-service does not otherwise touch Postgres, so the monitor is off unless RECONCILE_PG_DSN is set.
type Monitor struct {
	PG        *sql.DB
	CH        *ch.Client
	Days      int           // how many days back, including today (default 2)
	Tolerance float64       // allowed |ch-pg|/max(|pg|,1) per day and metric (default 0.02: today's events are still in flight)
	Interval  time.Duration // time between runs (default 10m)
	Now       func() time.Time

	mu       sync.RWMutex
	last     *Report
	lastErr  string
	lastRun  time.Time
	lastOK   time.Time // last run that completed (even with mismatches)
	runs     int64
	failures int64 // runs that could not complete (database error)
}

// State is a copy of the monitor's last result.
type State struct {
	Report      *Report    `json:"report,omitempty"`
	Error       string     `json:"error,omitempty"`
	LastRun     *time.Time `json:"last_run,omitempty"`
	LastSuccess *time.Time `json:"last_success,omitempty"`
	Runs        int64      `json:"runs"`
	Failures    int64      `json:"failures"`
	Enabled     bool       `json:"enabled"`
}

func (m *Monitor) defaults() {
	if m.Days <= 0 {
		m.Days = 2
	}
	if m.Tolerance < 0 {
		m.Tolerance = 0
	}
	if m.Interval <= 0 {
		m.Interval = 10 * time.Minute
	}
	if m.Now == nil {
		m.Now = time.Now
	}
}

// RunOnce reconciles the last Days days and stores the result.
func (m *Monitor) RunOnce(ctx context.Context) {
	m.defaults()
	today := m.Now().In(mustLoc())
	from := today.AddDate(0, 0, -(m.Days - 1)).Format("2006-01-02")
	to := today.Format("2006-01-02")
	r, err := ParseDays(from, to)
	var rep *Report
	if err == nil {
		rep, err = Reconcile(ctx, m.PG, m.CH, r, m.Tolerance)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs++
	m.lastRun = m.Now()
	if err != nil {
		m.failures++
		m.lastErr = err.Error()
		return
	}
	m.last, m.lastErr, m.lastOK = rep, "", m.Now()
}

// Run blocks until ctx is done, reconciling at startup and then every Interval.
func (m *Monitor) Run(ctx context.Context) {
	m.defaults()
	m.RunOnce(ctx)
	t := time.NewTicker(m.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.RunOnce(ctx)
		}
	}
}

// Snapshot returns the last result (nil-safe: a nil Monitor reports Enabled=false).
func (m *Monitor) Snapshot() State {
	if m == nil {
		return State{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := State{Report: m.last, Error: m.lastErr, Runs: m.runs, Failures: m.failures, Enabled: true}
	if !m.lastRun.IsZero() {
		t := m.lastRun
		s.LastRun = &t
	}
	if !m.lastOK.IsZero() {
		t := m.lastOK
		s.LastSuccess = &t
	}
	if s.Report != nil {
		// the per-day lines can be long; the health report keeps the totals and only the mismatching days
		slim := *s.Report
		slim.Lines = nil
		for _, l := range s.Report.Lines {
			if l.Mismatch {
				slim.Lines = append(slim.Lines, l)
			}
		}
		s.Report = &slim
	}
	return s
}

// MaxRatio is mm_reconcile_diff_ratio (-1 until the first successful run).
func (m *Monitor) MaxRatio() float64 {
	s := m.Snapshot()
	if s.Report == nil {
		return -1
	}
	return s.Report.MaxRatio
}
