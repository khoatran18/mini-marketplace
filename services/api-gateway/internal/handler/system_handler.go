package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SystemTarget is one service whose admin /ready endpoint the gateway aggregates.
type SystemTarget struct {
	Name string // display name, e.g. "order-service"
	URL  string // admin base URL, e.g. "http://order-service:8081"
}

// SystemHandler serves admin-only operational endpoints.
type SystemHandler struct {
	Targets []SystemTarget
	Client  *http.Client
	Logger  *zap.Logger
}

// NewSystemHandler builds a SystemHandler with a short per-request timeout.
func NewSystemHandler(targets []SystemTarget, logger *zap.Logger) *SystemHandler {
	return &SystemHandler{Targets: targets, Client: &http.Client{Timeout: 2 * time.Second}, Logger: logger}
}

type serviceHealth struct {
	Name      string          `json:"name"`
	Status    string          `json:"status"` // ready | degraded | not_ready | unreachable
	LatencyMs int64           `json:"latency_ms"`
	Version   string          `json:"version,omitempty"`
	UptimeS   int64           `json:"uptime_s,omitempty"`
	Checks    json.RawMessage `json:"checks,omitempty"`
}

type systemHealth struct {
	AsOf     time.Time       `json:"as_of"`
	Status   string          `json:"status"` // worst status across services
	Services []serviceHealth `json:"services"`
}

// Health godoc
// @Summary Aggregated /ready of every service
// @Description Admin only. Calls each service's internal /ready endpoint and returns one table.
// @Tags admin
// @Security BearerAuth
// @Success 200 {object} systemHealth
// @Router /admin/system/health [get]
func (h *SystemHandler) Health(c *gin.Context) {
	results := make([]serviceHealth, len(h.Targets))
	var wg sync.WaitGroup
	for i, t := range h.Targets {
		wg.Add(1)
		go func(i int, t SystemTarget) {
			defer wg.Done()
			results[i] = h.probe(c.Request.Context(), t)
		}(i, t)
	}
	wg.Wait()
	sort.Slice(results, func(a, b int) bool { return results[a].Name < results[b].Name })

	rank := map[string]int{"ready": 0, "degraded": 1, "not_ready": 2, "unreachable": 3}
	worst := "ready"
	for _, r := range results {
		if rank[r.Status] > rank[worst] {
			worst = r.Status
		}
	}
	c.JSON(http.StatusOK, systemHealth{AsOf: time.Now().UTC(), Status: worst, Services: results})
}

func (h *SystemHandler) probe(ctx context.Context, t SystemTarget) serviceHealth {
	start := time.Now()
	out := serviceHealth{Name: t.Name, Status: "unreachable"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL+"/ready", nil)
	if err != nil {
		return out
	}
	resp, err := h.Client.Do(req)
	out.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		h.Logger.Warn("SystemHandler: service unreachable", zap.String("service", t.Name))
		return out
	}
	defer resp.Body.Close()
	var body struct {
		Status  string          `json:"status"`
		Version string          `json:"version"`
		UptimeS int64           `json:"uptime_s"`
		Checks  json.RawMessage `json:"checks"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil || body.Status == "" {
		return out
	}
	out.Status, out.Version, out.UptimeS, out.Checks = body.Status, body.Version, body.UptimeS, body.Checks
	return out
}
