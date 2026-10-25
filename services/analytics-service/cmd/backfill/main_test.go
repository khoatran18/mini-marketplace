package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"nope"},
		{"backfill"},
		{"backfill", "--from", "2026-01-01", "--to", "bad"},
		{"backfill", "--from", "2026-01-01", "--to", "2026-01-02", "--tables", "orders"},
		{"reconcile", "--from", "2026-01-02", "--to", "2026-01-01"},
		{"reconcile", "--from", "2026-01-01", "--to", "2026-01-02", "--tolerance", "-1"},
		{"reconcile", "--from", "2026-01-01", "--to", "2026-01-02", "--postgres-dsn", ""}, // no DSN
	} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 {
			t.Errorf("run(%v) = %d want 2 (stderr %q)", args, code, errb.String())
		}
	}
	var out, errb bytes.Buffer
	run([]string{"reconcile", "--from", "2026-01-01", "--to", "2026-01-02", "--postgres-dsn", ""}, &out, &errb)
	if !strings.Contains(errb.String(), "postgres-dsn") {
		t.Errorf("stderr: %q", errb.String())
	}
}
