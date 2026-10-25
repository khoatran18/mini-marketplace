package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"analytics-service/internal/ch"
)

func newSpoolSink(t *testing.T, f *fakeCH, flushRows, bufMax int, spoolMax int64) (*Sink, *Spool, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	sp, err := NewSpool(t.TempDir(), spoolMax)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSink(ch.New(srv.URL, "d", "", ""), time.Hour, flushRows, bufMax).WithSpool(sp)
	return s, sp, srv.Close
}

func TestSpoolOverflowIsSpooledNotDroppedAndReplayed(t *testing.T) {
	f := &fakeCH{tables: map[string]int{}, fail: true}
	s, sp, closeFn := newSpoolSink(t, f, 3, 5, 0)
	defer closeFn()
	for i := 0; i < 8; i++ {
		s.Add("a", map[string]int{"x": i})
	}
	st := s.Stats()
	if st.Buffered != 5 || st.Spooled != 3 || st.Dropped != 0 || st.SpoolBytes == 0 {
		t.Fatalf("%+v", st)
	}
	// outage ends: replay inserts the spooled rows and removes the file
	f.mu.Lock()
	f.fail = false
	f.mu.Unlock()
	s.Replay(context.Background())
	if f.tables["a"] != 3 || s.Stats().Replayed != 3 || sp.Size() != 0 {
		t.Fatalf("%v %+v", f.tables, s.Stats())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(sp.path), spoolFile)); !os.IsNotExist(err) {
		t.Fatalf("spool file should be gone: %v", err)
	}
}

func TestSpoolReplayFailureKeepsRows(t *testing.T) {
	f := &fakeCH{tables: map[string]int{}, fail: true}
	s, sp, closeFn := newSpoolSink(t, f, 3, 3, 0)
	defer closeFn()
	for i := 0; i < 5; i++ {
		s.Add("a", map[string]int{"x": i})
	}
	before := sp.Size()
	s.Replay(context.Background())
	if sp.Size() != before || s.Stats().Replayed != 0 {
		t.Fatalf("rows must stay spooled: %d vs %d %+v", sp.Size(), before, s.Stats())
	}
}

func TestSpoolSizeCapDrops(t *testing.T) {
	f := &fakeCH{tables: map[string]int{}, fail: true}
	s, _, closeFn := newSpoolSink(t, f, 3, 3, 60) // room for ~2 small lines
	defer closeFn()
	for i := 0; i < 10; i++ {
		s.Add("a", map[string]int{"x": i})
	}
	st := s.Stats()
	if st.Spooled == 0 || st.Dropped == 0 || st.Spooled+st.Dropped != 7 || st.SpoolBytes > 60 {
		t.Fatalf("%+v", st)
	}
}

func TestShutdownSpoolsBufferAndRestartReplays(t *testing.T) {
	f := &fakeCH{tables: map[string]int{}, fail: true}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()
	dir := t.TempDir()
	sp, _ := NewSpool(dir, 0)
	s := NewSink(ch.New(srv.URL, "d", "", ""), 10*time.Millisecond, 1000, 5000).WithSpool(sp)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	s.Add("a", map[string]int{"x": 1})
	s.Add("b", map[string]int{"x": 2})
	cancel() // SIGTERM path: ClickHouse is down, so the buffer must land in the spool
	<-done
	if st := s.Stats(); st.Buffered != 0 || st.Spooled != 2 {
		t.Fatalf("%+v", st)
	}

	// "restart": a new sink on the same dir replays at startup
	f.mu.Lock()
	f.fail = false
	f.mu.Unlock()
	sp2, _ := NewSpool(dir, 0)
	s2 := NewSink(ch.New(srv.URL, "d", "", ""), 10*time.Millisecond, 1000, 5000).WithSpool(sp2)
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() { s2.Run(ctx2); close(done2) }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s2.Stats().Replayed < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	cancel2()
	<-done2
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tables["a"] != 1 || f.tables["b"] != 1 || sp2.Size() != 0 {
		t.Fatalf("%v size=%d", f.tables, sp2.Size())
	}
}

func TestSpoolSkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	sp, _ := NewSpool(dir, 0)
	if _, err := sp.Append([]pending{{table: "a", row: map[string]int{"x": 1}}}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(sp.path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("{\"t\":\"a\",\"r\":{\"x\"") // torn write
	f.Close()
	recs, corrupt, err := sp.readAll()
	if err != nil || len(recs) != 1 || corrupt != 1 {
		t.Fatalf("%v %d %v", recs, corrupt, err)
	}
}
