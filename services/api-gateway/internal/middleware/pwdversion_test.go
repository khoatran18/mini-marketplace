package middleware

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type fakeSrc struct {
	version int64
	err     error
	calls   int
}

func (f *fakeSrc) GetPwdVersion(context.Context, uint64) (int64, error) {
	f.calls++
	return f.version, f.err
}

const window = 6 * time.Minute

func fbEngine(mr *miniredis.Miniredis, src PwdVersionSource) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	rdb := redisFor(mr)
	r.GET("/me", AuthMiddleware(zap.NewNop(), rdb, secret, WithPwdVersionFallback(src, window)), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	return r
}

func markSafe(mr *miniredis.Miniredis) { // Redis "đã chạy đủ lâu": cửa sổ an toàn đã qua
	_ = mr.Set(pwdSafeAfterKey, strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10))
}

func TestRedisDownFallsBackToDatabase(t *testing.T) {
	_, mr := newRedis(t)
	src := &fakeSrc{version: 0}
	e := fbEngine(mr, src)
	mr.Close() // Redis chết
	tok := sign(t, claims("access", 0, time.Minute), secret)
	if w := get(e, tok); w.Code != 200 {
		t.Fatalf("Redis chết, DB nói version khớp: want 200, got %d", w.Code)
	}
	src.version = 1 // đã đổi mật khẩu trong DB
	if w := get(e, tok); w.Code != 401 {
		t.Fatalf("Redis chết, DB nói đã đổi mật khẩu: want 401, got %d", w.Code)
	}
}

func TestBothDownFailsClosedWith503(t *testing.T) {
	_, mr := newRedis(t)
	src := &fakeSrc{err: errors.New("auth-service down")}
	e := fbEngine(mr, src)
	mr.Close()
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 503 {
		t.Fatalf("Redis và DB cùng lỗi phải từ chối bằng 503 (không phải 401 để client không tự đăng xuất), got %d", w.Code)
	}
}

func TestFreshOrFlushedRedisIsNotTrusted(t *testing.T) {
	_, mr := newRedis(t)
	src := &fakeSrc{version: 1} // mật khẩu đã đổi, nhưng Redis (mới/xoá sạch) không còn khoá deny
	e := fbEngine(mr, src)
	old := sign(t, claims("access", 0, time.Minute), secret) // vé cấp trước khi đổi mật khẩu
	if w := get(e, old); w.Code != 401 {
		t.Fatalf("Redis mất dữ liệu: vé cũ phải bị DB chặn, got %d", w.Code)
	}
	if src.calls != 1 {
		t.Fatalf("phải hỏi DB, calls=%d", src.calls)
	}
	v, err := mr.Get(pwdSafeAfterKey)
	if err != nil {
		t.Fatal("phải tạo khoá cửa sổ an toàn")
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	if d := time.Until(time.Unix(n, 0)); d < window-10*time.Second || d > window+10*time.Second {
		t.Fatalf("cửa sổ an toàn ~%v, got %v", window, d)
	}
	if mr.TTL(pwdSafeAfterKey) != 0 {
		t.Fatal("khoá cửa sổ không được có TTL")
	}
	// trong cửa sổ, vé mới (version 1) vẫn qua nhờ DB
	if w := get(e, sign(t, claims("access", 1, time.Minute), secret)); w.Code != 200 {
		t.Fatalf("vé mới hợp lệ, got %d", w.Code)
	}
}

func TestAfterWindowRedisIsTrustedAndDatabaseNotCalled(t *testing.T) {
	_, mr := newRedis(t)
	src := &fakeSrc{err: errors.New("không được gọi")}
	e := fbEngine(mr, src)
	markSafe(mr)
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 200 {
		t.Fatalf("không có khoá deny: want 200, got %d", w.Code)
	}
	_ = mr.Set("42:pwd_version", "1") // vừa đổi mật khẩu (consumer ghi)
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 401 {
		t.Fatalf("vé cũ sau khi đổi mật khẩu: want 401, got %d", w.Code)
	}
	if w := get(e, sign(t, claims("access", 1, time.Minute), secret)); w.Code != 200 {
		t.Fatalf("vé mới: want 200, got %d", w.Code)
	}
	if src.calls != 0 {
		t.Fatalf("đường nhanh không được gọi DB, calls=%d", src.calls)
	}
}

func redisFor(mr *miniredis.Miniredis) *redis.Client { return redis.NewClient(&redis.Options{Addr: mr.Addr()}) }
