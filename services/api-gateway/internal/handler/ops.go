package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// OpsHandler proxies /admin/ops/* to ops-service. The gateway has already verified the JWT and the admin role; ops-service
// trusts only the shared bearer token, so it must never be reachable from outside the overlay network.
type OpsHandler struct {
	BaseURL string
	Token   string
	Client  *http.Client
	Logger  *zap.Logger
}

// NewOpsHandler builds the proxy. An empty token disables it (503) so a missing secret never opens the upstream.
func NewOpsHandler(baseURL, token string, logger *zap.Logger) *OpsHandler {
	return &OpsHandler{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, Client: &http.Client{Timeout: 5 * time.Second}, Logger: logger}
}

// Proxy forwards the request to upstreamPath, filling ":id" from the route. JSON bodies up to 64 KB are forwarded.
func (h *OpsHandler) Proxy(upstreamPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.Token == "" || h.BaseURL == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ops service is not configured"})
			return
		}
		path := upstreamPath
		if strings.Contains(path, ":id") {
			id := c.Param("id")
			for _, r := range id {
				if r < '0' || r > '9' {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
					return
				}
			}
			path = strings.Replace(path, ":id", id, 1)
		}
		if strings.Contains(path, ":name") {
			name := c.Param("name")
			if name == "" || len(name) > 64 || strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query name"})
				return
			}
			path = strings.Replace(path, ":name", name, 1)
		}
		if strings.Contains(path, ":service") {
			svc := c.Param("service")
			if svc == "" || len(svc) > 32 || strings.Trim(svc, "abcdefghijklmnopqrstuvwxyz-") != "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service"})
				return
			}
			path = strings.Replace(path, ":service", svc, 1)
		}
		url := h.BaseURL + path
		if q := c.Request.URL.RawQuery; q != "" {
			url += "?" + q
		}
		var body io.Reader
		if c.Request.Method == http.MethodPost && c.Request.Body != nil {
			b, err := io.ReadAll(io.LimitReader(c.Request.Body, (64<<10)+1))
			if err != nil || len(b) > 64<<10 {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
				return
			}
			body = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, url, body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+h.Token)
		role := "admin"
		if r, ok := c.Value("userRole").(string); ok && r != "" {
			role = r
		}
		req.Header.Set("X-Actor", fmt.Sprintf("%s:%v", role, c.Value("userID")))
		if rid, ok := c.Get("requestID"); ok {
			req.Header.Set("X-Request-ID", fmt.Sprint(rid))
		}
		resp, err := h.Client.Do(req)
		if err != nil {
			h.Logger.Warn("OpsHandler: upstream unavailable", zap.Error(err))
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ops service is unavailable"})
			return
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		c.Data(resp.StatusCode, "application/json", out)
	}
}
