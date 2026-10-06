package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"analytics-service/internal/ch"
)

type fakeCH struct {
	mu     sync.Mutex
	fail   bool
	tables map[string]int
}

func (f *fakeCH) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		http.Error(w, "down", 503)
		return
	}
	q := r.URL.Query().Get("query")
	table := strings.Fields(strings.TrimPrefix(q, "INSERT INTO "))[0]
	buf := make([]byte, 1<<16)
	n, _ := r.Body.Read(buf)
	f.tables[table] += strings.Count(string(buf[:n]), "\n")
}

func TestSinkBatchesRetriesAndDrops(t *testing.T) {
	f := &fakeCH{tables: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()
	s := NewSink(ch.New(srv.URL, "d", "", ""), time.Hour, 3, 5)
	ctx := context.Background()

	s.Add("a", map[string]int{"x": 1})
	s.Add("b", map[string]int{"x": 1})
	s.Add("a", map[string]int{"x": 2})
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if f.tables["a"] != 2 || f.tables["b"] != 1 || s.Stats().Buffered != 0 || s.Stats().Inserted != 3 {
		t.Fatalf("one batch per table: %v %+v", f.tables, s.Stats())
	}

	// outage: rows stay queued, nothing is lost, the failure is counted
	f.mu.Lock()
	f.fail = true
	f.mu.Unlock()
	s.Add("a", map[string]int{"x": 3})
	if err := s.Flush(ctx); err == nil {
		t.Fatal("flush must report the outage")
	}
	if st := s.Stats(); st.Buffered != 1 || st.FailedBatches != 1 || st.OldestAgeSeconds < 0 {
		t.Fatalf("%+v", st)
	}
	// recovery: the queued row goes out
	f.mu.Lock()
	f.fail = false
	f.mu.Unlock()
	if err := s.Flush(ctx); err != nil || f.tables["a"] != 3 || s.Stats().Buffered != 0 {
		t.Fatalf("%v %v %+v", err, f.tables, s.Stats())
	}

	// a long outage cannot exhaust memory: the oldest rows are dropped and counted
	f.mu.Lock()
	f.fail = true
	f.mu.Unlock()
	for i := 0; i < 8; i++ {
		s.Add("a", map[string]int{"x": i})
	}
	if st := s.Stats(); st.Buffered != 5 || st.Dropped != 3 {
		t.Fatalf("%+v", st)
	}
}

func TestSinkRunFlushesOnInterval(t *testing.T) {
	f := &fakeCH{tables: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()
	s := NewSink(ch.New(srv.URL, "d", "", ""), 20*time.Millisecond, 1000, 5000)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	s.Add("a", map[string]int{"x": 1})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := f.tables["a"]
		f.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.Add("a", map[string]int{"x": 2}) // flushed on shutdown
	cancel()
	<-done
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tables["a"] != 2 {
		t.Fatalf("rows: %v", f.tables)
	}
}
