package actions

import (
	"strings"
	"testing"
)

func TestUnknownActionRejected(t *testing.T) {
	if _, err := Validate("drop_database", nil); err == nil || !strings.Contains(err.Error(), "không có trong sổ") {
		t.Fatalf("hành động ngoài sổ phải bị từ chối: %v", err)
	}
}

func TestValidateParams(t *testing.T) {
	ok := []struct {
		id string
		p  map[string]any
	}{
		{"restart_service", map[string]any{"service": "order-service"}},
		{"scale_service", map[string]any{"service": "api-gateway", "replicas": float64(2)}},
		{"requeue_outbox", map[string]any{"service": "order-service", "ids": []any{float64(3), float64(1), float64(3)}}},
		{"requeue_outbox", map[string]any{"service": "payment-service", "limit": float64(10)}},
		{"run_backfill", map[string]any{"from": "2026-11-01", "to": "2026-11-12"}},
		{"escalate_to_human", nil},
	}
	for _, c := range ok {
		if _, err := Validate(c.id, c.p); err != nil {
			t.Errorf("%s %v: %v", c.id, c.p, err)
		}
	}
	bad := []struct {
		id   string
		p    map[string]any
		want string
	}{
		{"restart_service", map[string]any{"service": "postgres"}, "không nằm trong"},    // hạ tầng có trạng thái không restart được
		{"restart_service", map[string]any{"service": "ops-service"}, "không nằm trong"}, // không tự restart chính mình
		{"restart_service", map[string]any{}, "thiếu"},
		{"restart_service", map[string]any{"service": "order-service", "x": 1}, "tham số lạ"},
		{"scale_service", map[string]any{"service": "order-service", "replicas": float64(0)}, "ngoài khoảng"}, // scale về 0 không cho phép
		{"scale_service", map[string]any{"service": "order-service", "replicas": float64(1.5)}, "số nguyên"},
		{"requeue_outbox", map[string]any{"service": "order-service"}, "đúng một"},
		{"requeue_outbox", map[string]any{"service": "order-service", "ids": []any{float64(1)}, "limit": float64(5)}, "đúng một"},
		{"requeue_outbox", map[string]any{"service": "order-service", "limit": float64(1000)}, "ngoài khoảng"},
		{"requeue_outbox", map[string]any{"service": "order-service", "ids": []any{float64(-1)}}, "dương"},
		{"run_backfill", map[string]any{"from": "2026-11-12", "to": "2026-11-01"}, "không trước"},
		{"run_backfill", map[string]any{"from": "2026-01-01", "to": "2026-11-01"}, "31 ngày"},
		{"run_backfill", map[string]any{"from": "12/11/2026", "to": "2026-11-12"}, "YYYY-MM-DD"},
		{"escalate_to_human", map[string]any{"anything": "x"}, "tham số lạ"},
	}
	for _, c := range bad {
		_, err := Validate(c.id, c.p)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %v: muốn lỗi chứa %q, được %v", c.id, c.p, c.want, err)
		}
	}
}

func TestRisksAndHash(t *testing.T) {
	want := map[string]string{"requeue_outbox": RiskHigh, "run_backfill": RiskHigh, "restart_service": RiskLow, "scale_service": RiskLow, "escalate_to_human": RiskObserve}
	for id, r := range want {
		if a, _ := Get(id); a.Risk != r {
			t.Errorf("%s risk %q, muốn %q", id, a.Risk, r)
		}
	}
	inc := int64(7)
	a := ContentHash(&inc, "restart_service", map[string]any{"service": "a", "x": 1}, RiskLow)
	b := ContentHash(&inc, "restart_service", map[string]any{"x": 1, "service": "a"}, RiskLow)
	if a != b {
		t.Fatal("thứ tự khoá không được đổi băm")
	}
	if a == ContentHash(&inc, "restart_service", map[string]any{"service": "b", "x": 1}, RiskLow) || a == ContentHash(nil, "restart_service", map[string]any{"service": "a", "x": 1}, RiskLow) || a == ContentHash(&inc, "restart_service", map[string]any{"service": "a", "x": 1}, RiskHigh) {
		t.Fatal("nội dung khác phải băm khác")
	}
}
