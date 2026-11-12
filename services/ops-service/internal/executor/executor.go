// Package executor tự chạy các hành động mode=http của sổ hành động sau khi người đã duyệt. Mỗi hành động có tiền điều kiện được kiểm
// TRƯỚC khi ghi và kiểm lại SAU khi ghi. Không có quyền docker/shell: hành động mode=command chỉ được sinh lệnh cho người chạy.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrPrecondition: tiền điều kiện không đạt, chưa có thay đổi nào được thực hiện.
var ErrPrecondition = errors.New("precondition failed")

// HTTP gọi cổng admin nội bộ (:8081) của service đích. Token là METRICS_TOKEN (nếu service bật).
type HTTP struct {
	Client  *http.Client
	BaseURL func(service string) string
	Token   string
}

func (h *HTTP) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (h *HTTP) base(service string) string {
	if h.BaseURL != nil {
		return h.BaseURL(service)
	}
	return "http://" + service + ":8081"
}

func (h *HTTP) do(ctx context.Context, method, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return err
	}
	if h.Token != "" {
		req.Header.Set("Authorization", "Bearer "+h.Token)
	}
	resp, err := h.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s trả %d", method, req.URL.Path, resp.StatusCode)
	}
	return json.Unmarshal(b, out)
}

type deadList struct {
	Total int `json:"dead_total"`
	Rows  []struct {
		ID uint64 `json:"id"`
	} `json:"rows"`
}

// Run thực thi hành động. Params đã được actions.Validate (số có thể là float64 hoặc int). Kết quả trả về được lưu vào exec_result.
func (h *HTTP) Run(ctx context.Context, actionID string, params map[string]any) (map[string]any, error) {
	switch actionID {
	case "requeue_outbox":
		return h.requeueOutbox(ctx, params)
	}
	return nil, fmt.Errorf("hành động %q không tự chạy được", actionID)
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func (h *HTTP) requeueOutbox(ctx context.Context, p map[string]any) (map[string]any, error) {
	svc, _ := p["service"].(string)
	if svc == "" {
		return nil, fmt.Errorf("%w: thiếu service", ErrPrecondition)
	}
	base := h.base(svc)
	var before deadList
	if err := h.do(ctx, http.MethodGet, base+"/internal/outbox/dead?limit=100", &before); err != nil {
		return nil, fmt.Errorf("%w: không đọc được danh sách DEAD: %v", ErrPrecondition, err)
	}
	if before.Total == 0 {
		return nil, fmt.Errorf("%w: không có hàng DEAD nào", ErrPrecondition)
	}
	q := url.Values{}
	if raw, ok := p["ids"]; ok {
		have := map[uint64]bool{}
		for _, r := range before.Rows {
			have[r.ID] = true
		}
		var ids []int
		switch l := raw.(type) {
		case []int:
			ids = l
		case []any:
			for _, x := range l {
				ids = append(ids, toInt(x))
			}
		}
		for _, id := range ids {
			if !have[uint64(id)] {
				return nil, fmt.Errorf("%w: id %d không ở trạng thái DEAD (hoặc ngoài 100 hàng DEAD mới nhất)", ErrPrecondition, id)
			}
			q.Add("id", strconv.Itoa(id))
		}
	} else {
		q.Set("limit", strconv.Itoa(toInt(p["limit"])))
	}
	var out struct {
		Requeued int `json:"requeued"`
	}
	if err := h.do(ctx, http.MethodPost, base+"/internal/outbox/requeue?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	// kiểm lại sau khi ghi
	var after deadList
	verr := h.do(ctx, http.MethodGet, base+"/internal/outbox/dead?limit=1", &after)
	res := map[string]any{"service": svc, "dead_before": before.Total, "requeued": out.Requeued, "dead_after": after.Total, "verified": verr == nil && after.Total <= before.Total-out.Requeued}
	if out.Requeued == 0 {
		return res, errors.New("requeue không đổi hàng nào")
	}
	if verr != nil {
		return res, fmt.Errorf("đã requeue nhưng không kiểm lại được: %v", verr)
	}
	return res, nil
}

// Strings dùng cho log: che token.
func (h *HTTP) String() string {
	return "executor.HTTP{token:" + strings.Repeat("*", min(len(h.Token), 4)) + "}"
}
