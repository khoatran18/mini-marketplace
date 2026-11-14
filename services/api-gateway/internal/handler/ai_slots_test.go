package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestRedisSlotsAllowOneHolderPerUserAndReleaseOnlyOwn(t *testing.T) {
	mr := miniredis.RunT(t)
	s := RedisSlots{RDB: redis.NewClient(&redis.Options{Addr: mr.Addr()})}
	ctx := context.Background()
	tok, ok, err := s.Acquire(ctx, "7", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire: %v %v", ok, err)
	}
	if _, ok, _ := s.Acquire(ctx, "7", time.Minute); ok {
		t.Fatal("a second stream for the same user must be refused")
	}
	if _, ok, _ := s.Acquire(ctx, "8", time.Minute); !ok {
		t.Fatal("another user is not affected")
	}
	s.Release(ctx, "7", "someone-else")
	if _, ok, _ := s.Acquire(ctx, "7", time.Minute); ok {
		t.Fatal("a wrong token must not free the slot")
	}
	s.Release(ctx, "7", tok)
	if _, ok, _ := s.Acquire(ctx, "7", time.Minute); !ok {
		t.Fatal("the slot is free after release")
	}
	mr.FastForward(2 * time.Minute) // crashed holder: the TTL frees it
	if _, ok, _ := s.Acquire(ctx, "7", time.Minute); !ok {
		t.Fatal("the ttl must free an abandoned slot")
	}
}

type fakeSlots struct {
	held     bool
	err      error
	released int
}

func (f *fakeSlots) Acquire(context.Context, string, time.Duration) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	if f.held {
		return "", false, nil
	}
	f.held = true
	return "t", true, nil
}
func (f *fakeSlots) Release(context.Context, string, string) { f.held = false; f.released++ }

func TestChatRefusesASecondStreamAndFreesTheSlotAfterwards(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: done\ndata: {}\n\n"))
	}))
	defer up.Close()
	gin.SetMode(gin.TestMode)
	slots := &fakeSlots{}
	h := NewAIHandler(true, up.URL, time.Second, zap.NewNop())
	h.Slots = slots
	e := gin.New()
	e.POST("/ai/chat", func(c *gin.Context) { c.Set("userID", uint64(7)) }, h.Chat)
	post := func() int {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("POST", "/ai/chat", nil))
		return w.Code
	}

	slots.held = true // the user's earlier stream is still running
	if code := post(); code != http.StatusConflict {
		t.Errorf("second stream: %d, want 409", code)
	}
	slots.held = false
	if code := post(); code != 200 || slots.released != 1 || slots.held {
		t.Errorf("normal stream: %d released=%d held=%v", code, slots.released, slots.held)
	}
	slots.err = errors.New("redis down")
	if code := post(); code != http.StatusServiceUnavailable {
		t.Errorf("slot store down must refuse, got %d", code)
	}
}
