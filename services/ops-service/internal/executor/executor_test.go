package executor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fakeService(t *testing.T, dead []int, token string) (*httptest.Server, *[]string) {
	var calls []string
	var requeued int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == "GET":
			rows := ""
			for i, id := range dead {
				if i > 0 {
					rows += ","
				}
				rows += fmt.Sprintf(`{"id":%d}`, id)
			}
			fmt.Fprintf(w, `{"dead_total":%d,"rows":[%s]}`, len(dead)-requeued, rows)
		default:
			n := len(r.URL.Query()["id"])
			if n == 0 {
				n = len(dead)
			}
			requeued = n
			fmt.Fprintf(w, `{"requeued":%d}`, n)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func exec(srv *httptest.Server, token string) *HTTP {
	return &HTTP{BaseURL: func(string) string { return srv.URL }, Token: token}
}

func TestRequeueByIDs(t *testing.T) {
	srv, calls := fakeService(t, []int{7, 9, 11}, "tok")
	res, err := exec(srv, "tok").Run(context.Background(), "requeue_outbox", map[string]any{"service": "order-service", "ids": []any{float64(7), float64(9)}})
	if err != nil {
		t.Fatal(err)
	}
	if res["requeued"] != 2 || res["verified"] != true || res["dead_before"] != 3 || res["dead_after"] != 1 {
		t.Fatalf("kết quả: %v", res)
	}
	if len(*calls) != 3 || (*calls)[1] != "POST /internal/outbox/requeue?id=7&id=9" {
		t.Fatalf("lời gọi: %v", *calls)
	}
}

func TestPreconditionBlocksWrite(t *testing.T) {
	srv, calls := fakeService(t, []int{7}, "")
	_, err := exec(srv, "").Run(context.Background(), "requeue_outbox", map[string]any{"service": "order-service", "ids": []any{float64(8)}})
	if !errors.Is(err, ErrPrecondition) {
		t.Fatalf("id không DEAD phải bị chặn: %v", err)
	}
	for _, c := range *calls {
		if c[:4] == "POST" {
			t.Fatalf("không được ghi khi tiền điều kiện hỏng: %v", *calls)
		}
	}
	empty, _ := fakeService(t, nil, "")
	if _, err := exec(empty, "").Run(context.Background(), "requeue_outbox", map[string]any{"service": "order-service", "limit": 10}); !errors.Is(err, ErrPrecondition) {
		t.Fatalf("không có DEAD phải bị chặn: %v", err)
	}
}

func TestWrongTokenAndUnknownAction(t *testing.T) {
	srv, _ := fakeService(t, []int{1}, "tok")
	if _, err := exec(srv, "bad").Run(context.Background(), "requeue_outbox", map[string]any{"service": "order-service", "limit": 5}); !errors.Is(err, ErrPrecondition) {
		t.Fatalf("sai token: %v", err)
	}
	if _, err := exec(srv, "tok").Run(context.Background(), "restart_service", nil); err == nil {
		t.Fatal("hành động mode=command không được tự chạy")
	}
}
