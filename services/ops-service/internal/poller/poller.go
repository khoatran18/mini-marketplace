// Package poller đọc /api/v1/alerts của Prometheus theo chu kỳ và đưa vào store.
package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"ops-service/internal/store"
)

// Poller định kỳ đọc cảnh báo đang bắn.
type Poller struct {
	URL      string // vd http://prometheus:9090
	Interval time.Duration
	Store    *store.Store
	Client   *http.Client
	Logf     func(format string, args ...any)

	LastSuccess atomic.Int64 // unix giây của lần đọc thành công gần nhất
	Failures    atomic.Int64
}

type apiResp struct {
	Status string `json:"status"`
	Data   struct {
		Alerts []struct {
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			State       string            `json:"state"`
		} `json:"alerts"`
	} `json:"data"`
}

// Fetch đọc các cảnh báo đang `firing` (bỏ `pending`).
func (p *Poller) Fetch(ctx context.Context) ([]store.Alert, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.URL, "/")+"/api/v1/alerts", nil)
	if err != nil {
		return nil, err
	}
	c := p.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus alerts: HTTP %d", resp.StatusCode)
	}
	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if r.Status != "success" {
		return nil, fmt.Errorf("prometheus alerts: status %q", r.Status)
	}
	out := []store.Alert{}
	for _, a := range r.Data.Alerts {
		if a.State != "firing" {
			continue
		}
		out = append(out, store.Alert{Name: a.Labels["alertname"], Labels: a.Labels, Annotations: a.Annotations})
	}
	return out, nil
}

// Once chạy một vòng. Lỗi đọc Prometheus KHÔNG làm resolve sự cố nào (không biết gì thì không kết luận).
func (p *Poller) Once(ctx context.Context) error {
	alerts, err := p.Fetch(ctx)
	if err != nil {
		p.Failures.Add(1)
		return err
	}
	opened, resolved, err := p.Store.Observe(ctx, alerts, time.Now(), 2*p.Interval)
	if err != nil {
		p.Failures.Add(1)
		return err
	}
	p.LastSuccess.Store(time.Now().Unix())
	if (opened > 0 || resolved > 0) && p.Logf != nil {
		p.Logf("ops poller: firing=%d opened=%d resolved=%d", len(alerts), opened, resolved)
	}
	return nil
}

// Run chạy tới khi ctx kết thúc.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	for {
		if err := p.Once(ctx); err != nil && p.Logf != nil {
			p.Logf("ops poller: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
