package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// The AI evaluation set (services/ai-agent/evals) is checked offline against tools.snapshot.json. This test keeps that file equal to
// the real table: rename a tool or a param and it fails until you run `go run ./cmd/dump-agent-tools > ../ai-agent/evals/tools.snapshot.json`
// (from services/api-gateway) and fix the cases that no longer match.
func TestEvalToolSnapshotIsCurrent(t *testing.T) {
	want, err := json.MarshalIndent(map[string]any{"scopes": DefaultScopes, "actions": DefaultActions}, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../../ai-agent/evals/tools.snapshot.json")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("snapshot is not JSON: %v", err)
	}
	_ = json.Unmarshal(want, &b)
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if !bytes.Equal(x, y) {
		t.Fatal("services/ai-agent/evals/tools.snapshot.json is stale: regenerate it (see the comment above this test)")
	}
}
