// Package actions là sổ hành động cho phép: mọi đề xuất phải trỏ tới một hành động ở đây, tham số bị kiểm chặt,
// và mức rủi ro do sổ này quyết định (không nhận từ người tạo đề xuất). Đợt 2 chỉ lưu và duyệt; chưa thực thi gì.
package actions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Mức rủi ro (cùng nghĩa với annotation `action` của luật cảnh báo, RUNBOOK §13).
const (
	RiskObserve = "observe"
	RiskLow     = "low"
	RiskHigh    = "high"
)

// Cách thực thi (đợt 3): http = ops-service tự gọi endpoint nội bộ sau khi duyệt; command = chỉ sinh lệnh cho người chạy; manual = người làm.
const (
	ModeHTTP    = "http"
	ModeCommand = "command"
	ModeManual  = "manual"
)

// Param mô tả một tham số.
type Param struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"` // enum | int | int_list | date
	Required bool     `json:"required"`
	Enum     []string `json:"enum,omitempty"`
	Min      int      `json:"min,omitempty"`
	Max      int      `json:"max,omitempty"`
}

// Action là một hành động cho phép.
type Action struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Risk     string  `json:"risk"`
	Mode     string  `json:"mode"`
	Params   []Param `json:"params"`
	Playbook string  `json:"playbook,omitempty"` // playbook / action gốc trong docs/playbooks
	validate func(map[string]any) error
}

var statelessServices = []string{"api-gateway", "auth-service", "user-service", "product-service", "order-service", "payment-service", "analytics-service"}

var catalog = []Action{
	{ID: "requeue_outbox", Title: "Requeue các hàng outbox DEAD (sau khi đã sửa nguyên nhân)", Risk: RiskHigh, Mode: ModeHTTP, Playbook: "outbox-stuck/inspect-then-requeue",
		Params: []Param{
			{Name: "service", Type: "enum", Required: true, Enum: []string{"order-service", "product-service", "payment-service"}},
			{Name: "ids", Type: "int_list", Min: 1, Max: 100},
			{Name: "limit", Type: "int", Min: 1, Max: 100},
		},
		validate: func(p map[string]any) error {
			_, hasIDs := p["ids"]
			_, hasLimit := p["limit"]
			if hasIDs == hasLimit {
				return errors.New("cần đúng một trong `ids` hoặc `limit`")
			}
			return nil
		}},
	{ID: "restart_service", Title: "Khởi động lại một service không trạng thái (docker service update --force)", Risk: RiskLow, Mode: ModeCommand, Playbook: "service-unavailable/force-restart",
		Params: []Param{{Name: "service", Type: "enum", Required: true, Enum: statelessServices}}},
	{ID: "scale_service", Title: "Đặt lại số replica của một service không trạng thái", Risk: RiskLow, Mode: ModeCommand, Playbook: "service-unavailable/scale-up",
		Params: []Param{
			{Name: "service", Type: "enum", Required: true, Enum: statelessServices},
			{Name: "replicas", Type: "int", Required: true, Min: 1, Max: 5},
		}},
	{ID: "run_backfill", Title: "Dựng lại dữ liệu ClickHouse từ Postgres cho một khoảng ngày (ghi đè)", Risk: RiskHigh, Mode: ModeCommand, Playbook: "analytics-data/backfill",
		Params: []Param{{Name: "from", Type: "date", Required: true}, {Name: "to", Type: "date", Required: true}},
		validate: func(p map[string]any) error {
			from, _ := time.Parse("2006-01-02", p["from"].(string))
			to, _ := time.Parse("2006-01-02", p["to"].(string))
			if to.Before(from) {
				return errors.New("`to` phải không trước `from`")
			}
			if to.Sub(from) > 31*24*time.Hour {
				return errors.New("khoảng ngày tối đa 31 ngày")
			}
			return nil
		}},
	{ID: "escalate_to_human", Title: "Báo người xử lý (không có thao tác tự động); dùng khi chưa khớp playbook nào", Risk: RiskObserve, Mode: ModeManual, Params: nil},
}

// All trả sổ hành động (bản sao, sắp theo id).
func All() []Action {
	out := append([]Action(nil), catalog...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get tìm hành động theo id.
func Get(id string) (Action, bool) {
	for _, a := range catalog {
		if a.ID == id {
			return a, true
		}
	}
	return Action{}, false
}

// Validate kiểm tham số theo lược đồ của hành động và trả về bản chuẩn hoá (số nguyên là int, khoá lạ bị từ chối).
func Validate(id string, params map[string]any) (map[string]any, error) {
	a, ok := Get(id)
	if !ok {
		return nil, fmt.Errorf("hành động %q không có trong sổ hành động cho phép", id)
	}
	if params == nil {
		params = map[string]any{}
	}
	known := map[string]Param{}
	for _, p := range a.Params {
		known[p.Name] = p
	}
	for k := range params {
		if _, ok := known[k]; !ok {
			return nil, fmt.Errorf("tham số lạ %q", k)
		}
	}
	out := map[string]any{}
	for _, p := range a.Params {
		v, present := params[p.Name]
		if !present {
			if p.Required {
				return nil, fmt.Errorf("thiếu tham số %q", p.Name)
			}
			continue
		}
		cv, err := convert(p, v)
		if err != nil {
			return nil, fmt.Errorf("tham số %q: %w", p.Name, err)
		}
		out[p.Name] = cv
	}
	if a.validate != nil {
		if err := a.validate(out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	}
	return 0, false
}

func convert(p Param, v any) (any, error) {
	switch p.Type {
	case "enum":
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("phải là chuỗi")
		}
		for _, e := range p.Enum {
			if e == s {
				return s, nil
			}
		}
		return nil, fmt.Errorf("giá trị %q không nằm trong %v", s, p.Enum)
	case "int":
		n, ok := asInt(v)
		if !ok {
			return nil, errors.New("phải là số nguyên")
		}
		if n < p.Min || n > p.Max {
			return nil, fmt.Errorf("ngoài khoảng %d..%d", p.Min, p.Max)
		}
		return n, nil
	case "int_list":
		l, ok := v.([]any)
		if !ok {
			return nil, errors.New("phải là danh sách số nguyên")
		}
		if len(l) < p.Min || len(l) > p.Max {
			return nil, fmt.Errorf("số phần tử ngoài khoảng %d..%d", p.Min, p.Max)
		}
		out := make([]int, 0, len(l))
		seen := map[int]bool{}
		for _, x := range l {
			n, ok := asInt(x)
			if !ok || n <= 0 {
				return nil, errors.New("mỗi phần tử phải là số nguyên dương")
			}
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
		sort.Ints(out)
		return out, nil
	case "date":
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("phải là chuỗi YYYY-MM-DD")
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, errors.New("phải có dạng YYYY-MM-DD")
		}
		return s, nil
	}
	return nil, fmt.Errorf("kiểu %q chưa hỗ trợ", p.Type)
}

// ContentHash là băm ổn định của nội dung cần duyệt (khoá map được json sắp xếp nên cùng nội dung cho cùng băm).
func ContentHash(incidentID *int64, actionID string, params map[string]any, risk string) string {
	b, _ := json.Marshal(struct {
		Incident int64          `json:"incident"`
		Action   string         `json:"action"`
		Params   map[string]any `json:"params"`
		Risk     string         `json:"risk"`
	}{func() int64 {
		if incidentID == nil {
			return 0
		}
		return *incidentID
	}(), actionID, params, risk})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Command dựng lệnh để NGƯỜI chạy cho hành động mode=command (ops-service không có quyền docker nên không tự chạy).
// Tham số đã qua Validate (enum/số/ngày) nên không chứa ký tự đặc biệt; trả "" nếu hành động không thuộc mode này.
func Command(id string, p map[string]any) string {
	switch id {
	case "restart_service":
		return fmt.Sprintf("docker service update --force --detach=true marketplace_%v", p["service"])
	case "scale_service":
		return fmt.Sprintf("docker service scale --detach=true marketplace_%v=%v", p["service"], p["replicas"])
	case "run_backfill":
		return fmt.Sprintf("(cd services/analytics-service && go run ./cmd/backfill backfill --from %v --to %v)", p["from"], p["to"])
	}
	return ""
}
