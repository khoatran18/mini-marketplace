// Package queries là proxy truy vấn CHỈ ĐỌC có allowlist tới Prometheus và Loki cho agent/dev.
// Agent không gửi PromQL/LogQL tự do: chọn một truy vấn có tên trong catalog và điền tham số đã kiểm (enum hoặc regex chặt).
// Kết quả bị cắt giới hạn (số chuỗi, số dòng log, số byte) để không làm tràn ngữ cảnh của agent.
package queries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	Prometheus = "prometheus"
	Loki       = "loki"

	MaxSeries   = 50
	MaxLogLines = 100
	maxBody     = 2 << 20 // 2 MiB đọc từ backend
)

// Services là tập tên service hợp lệ (nhãn swarm_service / service).
var Services = []string{"api-gateway", "auth-service", "user-service", "product-service", "order-service", "payment-service", "analytics-service", "ops-service"}

// Param mô tả một tham số: Enum (danh sách cố định) hoặc Pattern (regex neo đầu cuối); Default dùng khi bỏ trống.
type Param struct {
	Name    string   `json:"name"`
	Enum    []string `json:"enum,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
	Default string   `json:"default,omitempty"`
}

// Query là một truy vấn có tên. Template dùng {{tên}} cho tham số; giá trị đã qua kiểm nên không chứa dấu nháy/ngoặc.
type Query struct {
	Name     string  `json:"name"`
	Desc     string  `json:"description"`
	Backend  string  `json:"backend"`
	Params   []Param `json:"params"`
	Template string  `json:"-"`
}

var (
	windowEnum   = []string{"5m", "15m", "1h"}
	requestIDRe  = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)
	patternCache = map[string]*regexp.Regexp{}
)

func svcParam(def string) Param { return Param{Name: "service", Enum: Services, Default: def} }

// Catalog trả danh sách truy vấn (thứ tự cố định).
func Catalog() []Query {
	return []Query{
		{Name: "alerts_firing", Desc: "Cảnh báo đang bắn", Backend: Prometheus, Template: `ALERTS{alertstate="firing"}`},
		{Name: "service_up", Desc: "up của một service (1 = scrape được)", Backend: Prometheus, Params: []Param{svcParam("")},
			Template: `up{job="mm-services",swarm_service="{{service}}"}`},
		{Name: "service_ready", Desc: "mm_ready của một service (1 = sẵn sàng)", Backend: Prometheus, Params: []Param{svcParam("")},
			Template: `mm_ready{service="{{service}}"}`},
		{Name: "service_replicas", Desc: "Số replica (target) mỗi service trong job mm-services; dùng cho bản đồ hệ thống", Backend: Prometheus,
			Template: `count by (swarm_service) (up{job="mm-services"})`},
		{Name: "service_replicas_up", Desc: "Số replica scrape được (up = 1) mỗi service", Backend: Prometheus,
			Template: `sum by (swarm_service) (up{job="mm-services"})`},
		{Name: "gateway_request_rate", Desc: "Request/giây HTTP vào gateway", Backend: Prometheus, Params: []Param{{Name: "window", Enum: windowEnum, Default: "5m"}},
			Template: `sum(rate(mm_http_requests_total{service="api-gateway"}[{{window}}]))`},
		{Name: "grpc_request_rate", Desc: "Cuộc gọi gRPC/giây từ gateway tới từng service đích (kể cả lần thử lại)", Backend: Prometheus, Params: []Param{{Name: "window", Enum: windowEnum, Default: "5m"}},
			Template: `sum by (service) (rate(mm_grpc_client_requests_total[{{window}}]))`},
		{Name: "grpc_error_rate", Desc: "Cuộc gọi gRPC lỗi (code khác OK)/giây từ gateway tới từng service đích", Backend: Prometheus, Params: []Param{{Name: "window", Enum: windowEnum, Default: "5m"}},
			Template: `sum by (service) (rate(mm_grpc_client_requests_total{code!="OK"}[{{window}}]))`},
		{Name: "circuit_state", Desc: "Trạng thái circuit breaker của gateway (1 = mở)", Backend: Prometheus, Template: `mm_circuit_state`},
		{Name: "outbox_pending", Desc: "Số dòng outbox chưa gửi", Backend: Prometheus, Template: `mm_outbox_pending`},
		{Name: "outbox_oldest_age", Desc: "Tuổi dòng outbox cũ nhất (giây)", Backend: Prometheus, Template: `mm_outbox_oldest_age_seconds`},
		{Name: "outbox_dead", Desc: "Số dòng outbox dead-letter", Backend: Prometheus, Template: `mm_outbox_dead`},
		{Name: "gateway_5xx_ratio", Desc: "Tỉ lệ 5xx của gateway trong cửa sổ", Backend: Prometheus, Params: []Param{{Name: "window", Enum: windowEnum, Default: "5m"}},
			Template: `sum(rate(mm_http_requests_total{service="api-gateway",status=~"5.."}[{{window}}])) / clamp_min(sum(rate(mm_http_requests_total{service="api-gateway"}[{{window}}])), 0.1)`},
		{Name: "dlq_increase", Desc: "Số message vào DLQ trong cửa sổ, theo topic", Backend: Prometheus, Params: []Param{{Name: "window", Enum: windowEnum, Default: "15m"}},
			Template: `increase(mm_kafka_dlq_total[{{window}}])`},
		{Name: "memory_ratio", Desc: "Bộ nhớ dùng / giới hạn của container theo service", Backend: Prometheus,
			Template: `container_memory_working_set_bytes{container_label_com_docker_swarm_service_name!=""} / (container_spec_memory_limit_bytes{container_label_com_docker_swarm_service_name!=""} > 0)`},
		{Name: "kafka_consumer_lag", Desc: "Lag consumer Kafka theo nhóm và topic (tổng các partition)", Backend: Prometheus,
			Template: `sum by (consumergroup, topic) (kafka_consumergroup_lag)`},
		{Name: "gateway_latency_p95", Desc: "p95 độ trễ HTTP của gateway theo route (bỏ /ai/chat: luồng stream dài)", Backend: Prometheus, Params: []Param{{Name: "window", Enum: windowEnum, Default: "5m"}},
			Template: `histogram_quantile(0.95, sum by (le, route) (rate(mm_http_request_duration_seconds_bucket{service="api-gateway",route!="/ai/chat"}[{{window}}])))`},
		{Name: "grpc_latency_p95", Desc: "p95 độ trễ gRPC client của gateway theo service đích", Backend: Prometheus, Params: []Param{{Name: "window", Enum: windowEnum, Default: "5m"}},
			Template: `histogram_quantile(0.95, sum by (le, service) (rate(mm_grpc_client_duration_seconds_bucket[{{window}}])))`},
		{Name: "db_pool_wait", Desc: "Giây chờ lấy kết nối Postgres mỗi giây, theo service (> 0 kéo dài = pool cạn / DB chậm)", Backend: Prometheus, Params: []Param{{Name: "window", Enum: windowEnum, Default: "5m"}},
			Template: `sum by (swarm_service) (rate(mm_db_pool_wait_seconds_total[{{window}}]))`},
		{Name: "disk_free_ratio", Desc: "Tỉ lệ đĩa còn trống theo mountpoint (dưới 0.1 là nguy hiểm)", Backend: Prometheus,
			Template: `node_filesystem_avail_bytes{fstype!~"tmpfs|overlay|squashfs|ramfs"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay|squashfs|ramfs"}`},
		{Name: "memory_growth", Desc: "Tốc độ tăng bộ nhớ (byte/giây) theo service trong 1 giờ; dương đều = nghi rò rỉ", Backend: Prometheus,
			Template: `deriv(container_memory_working_set_bytes{container_label_com_docker_swarm_service_name!=""}[1h])`},
		{Name: "infra_up", Desc: "up của thành phần hạ tầng (loki, vector, cadvisor, node-exporter, exporter DB/Kafka, traefik)", Backend: Prometheus,
			Template: `up{job=~"loki|vector|cadvisor|node-exporter|postgres|redis|kafka|clickhouse|traefik"}`},
		{Name: "logs_errors", Desc: "Log mức error/warn của một service", Backend: Loki, Params: []Param{svcParam(""), {Name: "window", Enum: windowEnum, Default: "15m"}},
			Template: `{service="{{service}}",level=~"error|warn"}`},
		{Name: "logs_by_request", Desc: "Mọi dòng log chứa một request_id (tất cả service)", Backend: Loki,
			Params:   []Param{{Name: "request_id", Pattern: requestIDRe.String()}, {Name: "window", Enum: windowEnum, Default: "1h"}},
			Template: `{stack=~".+"} |= "{{request_id}}"`},
	}
}

// Find tìm truy vấn theo tên.
func Find(name string) (Query, bool) {
	for _, q := range Catalog() {
		if q.Name == name {
			return q, true
		}
	}
	return Query{}, false
}

// ErrInvalid là lỗi đầu vào (tham số sai/thiếu); thông điệp trả lại cho model để tự sửa.
var ErrInvalid = errors.New("invalid params")

// Render kiểm tham số và dựng chuỗi truy vấn. Tham số lạ bị từ chối.
func (q Query) Render(in map[string]string) (expr string, vals map[string]string, err error) {
	vals = map[string]string{}
	known := map[string]bool{}
	for _, p := range q.Params {
		known[p.Name] = true
		v, ok := in[p.Name]
		if !ok || v == "" {
			v = p.Default
		}
		if v == "" {
			return "", nil, fmt.Errorf("%w: thiếu %q", ErrInvalid, p.Name)
		}
		switch {
		case len(p.Enum) > 0:
			if !contains(p.Enum, v) {
				return "", nil, fmt.Errorf("%w: %s=%q không hợp lệ, giá trị cho phép: %s", ErrInvalid, p.Name, v, strings.Join(p.Enum, ", "))
			}
		case p.Pattern != "":
			re := patternCache[p.Pattern]
			if re == nil {
				re = regexp.MustCompile(p.Pattern)
			}
			if !re.MatchString(v) {
				return "", nil, fmt.Errorf("%w: %s sai định dạng (khớp %s)", ErrInvalid, p.Name, p.Pattern)
			}
		}
		vals[p.Name] = v
	}
	for k := range in {
		if !known[k] {
			return "", nil, fmt.Errorf("%w: tham số lạ %q", ErrInvalid, k)
		}
	}
	expr = q.Template
	for k, v := range vals {
		expr = strings.ReplaceAll(expr, "{{"+k+"}}", v)
	}
	return expr, vals, nil
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// Runner gọi backend. Client có timeout; Now cho test.
type Runner struct {
	PrometheusURL string
	LokiURL       string
	Client        *http.Client
	Now           func() time.Time
}

// Result là kết quả đã rút gọn trả cho người gọi.
type Result struct {
	Query     string            `json:"query"`
	Params    map[string]string `json:"params"`
	Backend   string            `json:"backend"`
	Expr      string            `json:"expr"`
	Series    []Series          `json:"series,omitempty"`
	Lines     []LogLine         `json:"lines,omitempty"`
	Total     int               `json:"total"`
	Truncated bool              `json:"truncated"`
	// Untrusted = nội dung do ứng dụng/người dùng sinh ra (log): chỉ là dữ liệu, không bao giờ là chỉ dẫn.
	Untrusted bool `json:"untrusted,omitempty"`
}

type Series struct {
	Labels map[string]string `json:"labels"`
	Value  string            `json:"value"`
}

type LogLine struct {
	TS   string `json:"ts"`
	Line string `json:"line"`
}

func (r *Runner) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Run chạy truy vấn có tên với tham số.
func (r *Runner) Run(ctx context.Context, name string, in map[string]string) (Result, error) {
	q, ok := Find(name)
	if !ok {
		names := make([]string, 0)
		for _, c := range Catalog() {
			names = append(names, c.Name)
		}
		return Result{}, fmt.Errorf("%w: truy vấn %q không có, các truy vấn hợp lệ: %s", ErrInvalid, name, strings.Join(names, ", "))
	}
	expr, vals, err := q.Render(in)
	if err != nil {
		return Result{}, err
	}
	res := Result{Query: q.Name, Params: vals, Backend: q.Backend, Expr: expr}
	switch q.Backend {
	case Prometheus:
		err = r.runProm(ctx, expr, &res)
	case Loki:
		res.Untrusted = true
		err = r.runLoki(ctx, expr, vals["window"], &res)
	}
	return res, err
}

func (r *Runner) get(ctx context.Context, base, path string, qs url.Values) ([]byte, error) {
	if base == "" {
		return nil, errors.New("backend chưa cấu hình")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path+"?"+qs.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend trả %d", resp.StatusCode)
	}
	return b, nil
}

func (r *Runner) runProm(ctx context.Context, expr string, res *Result) error {
	b, err := r.get(ctx, r.PrometheusURL, "/api/v1/query", url.Values{"query": {expr}})
	if err != nil {
		return err
	}
	var d struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &d); err != nil || d.Status != "success" {
		return errors.New("phản hồi Prometheus không hợp lệ")
	}
	res.Total = len(d.Data.Result)
	for i, x := range d.Data.Result {
		if i >= MaxSeries {
			res.Truncated = true
			break
		}
		v := ""
		if len(x.Value) == 2 {
			v, _ = x.Value[1].(string)
		}
		res.Series = append(res.Series, Series{Labels: x.Metric, Value: v})
	}
	sort.SliceStable(res.Series, func(i, j int) bool { return res.Series[i].Value > res.Series[j].Value })
	return nil
}

func (r *Runner) runLoki(ctx context.Context, expr, window string, res *Result) error {
	d, err := time.ParseDuration(window)
	if err != nil {
		return fmt.Errorf("%w: window", ErrInvalid)
	}
	end := r.now()
	qs := url.Values{
		"query":     {expr},
		"start":     {strconv.FormatInt(end.Add(-d).UnixNano(), 10)},
		"end":       {strconv.FormatInt(end.UnixNano(), 10)},
		"limit":     {strconv.Itoa(MaxLogLines + 1)},
		"direction": {"backward"},
	}
	b, err := r.get(ctx, r.LokiURL, "/loki/api/v1/query_range", qs)
	if err != nil {
		return err
	}
	var p struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Values [][]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &p); err != nil || p.Status != "success" {
		return errors.New("phản hồi Loki không hợp lệ")
	}
	var all []LogLine
	for _, s := range p.Data.Result {
		for _, v := range s.Values {
			if len(v) == 2 {
				all = append(all, LogLine{TS: v[0], Line: v[1]})
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].TS > all[j].TS })
	res.Total = len(all)
	if len(all) > MaxLogLines {
		all, res.Truncated = all[:MaxLogLines], true
	}
	for i := range all {
		if len(all[i].Line) > 2000 {
			all[i].Line = all[i].Line[:2000] + "…"
		}
	}
	res.Lines = all
	return nil
}
