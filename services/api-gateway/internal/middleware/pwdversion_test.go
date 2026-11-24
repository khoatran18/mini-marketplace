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
	src := &fakeSrc{version: 1}
	e := fbEngine(mr, src)
	markSafe(mr)
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 200 {
		t.Fatalf("không có khoá deny: want 200, got %d", w.Code)
	}
	_ = mr.Set("42:pwd_version", "1") // vừa đổi mật khẩu (consumer ghi)
	if w := get(e, sign(t, claims("access", 1, time.Minute), secret)); w.Code != 200 {
		t.Fatalf("vé mới: want 200, got %d", w.Code)
	}
	if src.calls != 0 {
		t.Fatalf("khoá khớp và hết cửa sổ: đường nhanh không được gọi DB, calls=%d", src.calls)
	}
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 401 {
		t.Fatalf("vé cũ sau khi đổi mật khẩu: want 401, got %d", w.Code)
	}
}

func TestStaleRedisKeyDoesNotRejectFreshToken(t *testing.T) {
	_, mr := newRedis(t)
	src := &fakeSrc{version: 2} // DB: đã đổi mật khẩu lần 2
	e := fbEngine(mr, src)
	markSafe(mr)
	_ = mr.Set("42:pwd_version", "1") // khoá cũ của lần đổi thứ nhất, sự kiện lần 2 chưa tới
	if w := get(e, sign(t, claims("access", 2, time.Minute), secret)); w.Code != 200 {
		t.Fatalf("vé mới sau lần đổi 2 phải qua nhờ DB dù khoá Redis cũ, got %d", w.Code)
	}
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 401 {
		t.Fatalf("vé cũ vẫn phải bị chặn, got %d", w.Code)
	}
}

func redisFor(mr *miniredis.Miniredis) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

// fakeRecent also lists recent changes, so a fresh Redis is rebuilt from the database instead of asking it on every request.
type fakeRecent struct {
	fakeSrc
	changes []PwdChange
	listErr error
	listed  int
	within  time.Duration
}

func (f *fakeRecent) ListRecentPwdChanges(_ context.Context, within time.Duration) ([]PwdChange, error) {
	f.listed++
	f.within = within
	return f.changes, f.listErr
}

func TestFreshRedisIsRebuiltFromDatabaseOnce(t *testing.T) {
	_, mr := newRedis(t)
	src := &fakeRecent{fakeSrc: fakeSrc{version: 1}, changes: []PwdChange{{UserID: 42, PwdVersion: 1}}}
	e := fbEngine(mr, src)
	// Redis mất dữ liệu, user 42 vừa đổi mật khẩu: vé cũ phải bị chặn ngay, vé mới qua.
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 401 {
		t.Fatalf("vé cũ sau khi Redis mất dữ liệu: want 401, got %d", w.Code)
	}
	if src.listed != 1 || src.within != window {
		t.Fatalf("phải hỏi danh sách đổi mật khẩu đúng 1 lần trong cửa sổ %v, listed=%d within=%v", window, src.listed, src.within)
	}
	if v, err := mr.Get("42:pwd_version"); err != nil || v != "1" {
		t.Fatalf("khoá deny phải được ghi lại, v=%q err=%v", v, err)
	}
	if ttl := mr.TTL("42:pwd_version"); ttl <= 0 || ttl > window {
		t.Fatalf("khoá deny phải có TTL <= cửa sổ, got %v", ttl)
	}
	s, err := mr.Get(pwdSafeAfterKey)
	if err != nil {
		t.Fatal("phải đặt khoá safe_after")
	}
	if n, _ := strconv.ParseInt(s, 10, 64); n > time.Now().Unix() {
		t.Fatalf("sau khi dựng lại Redis phải được tin NGAY (safe_after <= bây giờ), got +%ds", n-time.Now().Unix())
	}
	callsAfterRebuild := src.calls
	// Từ đây Redis được tin: vé mới khớp khoá, vé cũ bị khoá chặn, không hỏi DB.
	if w := get(e, sign(t, claims("access", 1, time.Minute), secret)); w.Code != 200 {
		t.Fatalf("vé mới: want 200, got %d", w.Code)
	}
	if src.calls != callsAfterRebuild || src.listed != 1 {
		t.Fatalf("vé khớp khoá: không được hỏi DB nữa: calls %d -> %d, listed=%d", callsAfterRebuild, src.calls, src.listed)
	}
	// Vé cũ lệch khoá: đường hiện có để DB quyết định (khoá có thể cũ), vẫn phải bị chặn.
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 401 {
		t.Fatalf("vé cũ: want 401, got %d", w.Code)
	}
	if src.listed != 1 {
		t.Fatalf("không dựng lại lần hai, listed=%d", src.listed)
	}
}

func TestFreshRedisWithNoRecentChangeAsksDatabaseOnlyOnce(t *testing.T) {
	_, mr := newRedis(t)
	src := &fakeRecent{fakeSrc: fakeSrc{version: 0}}
	e := fbEngine(mr, src)
	for i := 0; i < 5; i++ {
		if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 200 {
			t.Fatalf("lần %d: want 200, got %d", i, w.Code)
		}
	}
	if src.calls != 1 || src.listed != 1 {
		t.Fatalf("không ai đổi mật khẩu: chỉ request dựng lại hỏi DB 1 lần, calls=%d listed=%d", src.calls, src.listed)
	}
}

func TestRebuildDoesNotOverwriteNewerKey(t *testing.T) {
	_, mr := newRedis(t)
	_ = mr.Set("42:pwd_version", "2") // consumer Kafka đã ghi lần đổi thứ 2 trước khi dựng lại
	src := &fakeRecent{fakeSrc: fakeSrc{version: 2}, changes: []PwdChange{{UserID: 42, PwdVersion: 1}}}
	e := fbEngine(mr, src)
	get(e, sign(t, claims("access", 2, time.Minute), secret))
	if v, _ := mr.Get("42:pwd_version"); v != "2" {
		t.Fatalf("dựng lại không được ghi đè giá trị mới hơn, got %q", v)
	}
}

func TestRebuildFailureFallsBackToWindow(t *testing.T) {
	_, mr := newRedis(t)
	src := &fakeRecent{fakeSrc: fakeSrc{version: 0}, listErr: errors.New("auth-service down")}
	e := fbEngine(mr, src)
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 200 {
		t.Fatalf("danh sách lỗi nhưng DB đọc từng user vẫn được: want 200, got %d", w.Code)
	}
	s, err := mr.Get(pwdSafeAfterKey)
	if err != nil {
		t.Fatal("dựng lại lỗi phải quay về cửa sổ an toàn")
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	if d := time.Until(time.Unix(n, 0)); d < window-10*time.Second {
		t.Fatalf("cửa sổ ~%v, got %v", window, d)
	}
	if mr.Exists(pwdRebuildLockKey) {
		t.Fatal("khoá dựng lại phải được nhả")
	}
}

func TestRebuildBusyCreatesNoSentinel(t *testing.T) {
	_, mr := newRedis(t)
	_ = mr.Set(pwdRebuildLockKey, "1") // request khác đang dựng lại
	src := &fakeRecent{fakeSrc: fakeSrc{version: 0}}
	e := fbEngine(mr, src)
	if w := get(e, sign(t, claims("access", 0, time.Minute), secret)); w.Code != 200 {
		t.Fatalf("want 200 qua DB, got %d", w.Code)
	}
	if mr.Exists(pwdSafeAfterKey) {
		t.Fatal("request không giữ khoá không được tạo safe_after (sẽ chặn sentinel của lần dựng lại)")
	}
	if src.listed != 0 || src.calls != 1 {
		t.Fatalf("listed=%d calls=%d", src.listed, src.calls)
	}
}
