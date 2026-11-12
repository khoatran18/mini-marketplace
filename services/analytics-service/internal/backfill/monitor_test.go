package backfill

import (
	"testing"
	"time"
)

func TestMonitorSnapshotNilAndSlim(t *testing.T) {
	var nilMon *Monitor
	if nilMon.Snapshot().Enabled {
		t.Fatal("nil monitor must report disabled")
	}
	m := &Monitor{}
	if m.MaxRatio() != -1 {
		t.Fatalf("MaxRatio before first run = %v, want -1", m.MaxRatio())
	}
	now := time.Now()
	m.last = &Report{MaxRatio: 0.5, Mismatches: 1, Checked: 3,
		Lines: []Line{{Day: "2026-11-11", Metric: "a", Mismatch: false}, {Day: "2026-11-12", Metric: "b", Mismatch: true}}}
	m.lastOK, m.lastRun, m.runs = now, now, 2
	s := m.Snapshot()
	if !s.Enabled || s.Runs != 2 || s.LastSuccess == nil || s.Report == nil {
		t.Fatalf("%+v", s)
	}
	if len(s.Report.Lines) != 1 || s.Report.Lines[0].Metric != "b" {
		t.Fatalf("health report must keep only mismatching days: %+v", s.Report.Lines)
	}
	if len(m.last.Lines) != 2 {
		t.Fatal("Snapshot must not modify the stored report")
	}
	if m.MaxRatio() != 0.5 {
		t.Fatalf("MaxRatio = %v", m.MaxRatio())
	}
}
