package actions

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestAgentEvalSnapshotIsCurrent: bộ eval RCA của ai-agent dùng một bản sao của sổ hành động (không cần chạy ops-service).
// Sổ đổi mà bản sao không đổi thì eval chấm theo một sổ cũ. Cập nhật: UPDATE_SNAPSHOT=1 go test ./internal/actions -run Snapshot
func TestAgentEvalSnapshotIsCurrent(t *testing.T) {
	want, err := json.MarshalIndent(map[string]any{"actions": All()}, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	path := filepath.Join("..", "..", "..", "ai-agent", "evals", "rca", "actions.snapshot.json")
	if os.Getenv("UPDATE_SNAPSHOT") == "1" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("thiếu %s: chạy UPDATE_SNAPSHOT=1 go test ./internal/actions -run Snapshot", path)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s đã cũ so với sổ hành động: chạy UPDATE_SNAPSHOT=1 go test ./internal/actions -run Snapshot", path)
	}
}
