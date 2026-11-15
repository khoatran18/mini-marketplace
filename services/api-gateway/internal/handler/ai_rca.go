package handler

import (
	"api-gateway/internal/agent"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	// SystemTokenTTL is how long the RCA watcher's token lives; the watcher asks for a new one when it expires (ADR-47).
	SystemTokenTTL = 15 * time.Minute
	// SystemName is the only system identity there is; it appears in the token (`sys`) and in the log of every call it makes.
	SystemName = "rca-watcher"
	rcaMaxBody = 16 << 10
)

// SystemTokenHandler serves POST /ai/system-token (ADR-47): ai-agent's RCA watcher proves it knows AI_SYSTEM_KEY (header X-System-Key)
// and gets a read-only token that belongs to no account. Empty key = the route does not exist (404). The key is compared in constant
// time; the route is rate limited per IP at the router.
type SystemTokenHandler struct {
	Signer *agent.Signer
	Key    string
	Logger *zap.Logger
}

func (h SystemTokenHandler) Issue(c *gin.Context) {
	if h.Key == "" || h.Signer == nil || !h.Signer.Keys.Enabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	// hash both sides first so the comparison does not depend on the length of what the caller sent
	got, want := sha256.Sum256([]byte(c.GetHeader("X-System-Key"))), sha256.Sum256([]byte(h.Key))
	if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
		h.Logger.Warn("SystemTokenHandler: wrong system key", zap.String("ip", c.ClientIP()))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid system key"})
		return
	}
	tok, jti, err := h.Signer.IssueSystemToken(SystemName, SystemTokenTTL)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cannot issue the token"})
		return
	}
	h.Logger.Info("system token issued", zap.String("agent_run_id", jti), zap.String("sys", SystemName))
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"token": tok, "run_id": jti, "expires_in": int(SystemTokenTTL.Seconds())})
}

// Forward proxies one RCA route to ai-agent (`/rca/...`, ADR-46/47) with a 60 s agent token made from the verified JWT: ai-agent only
// reads who and which role from it. upstream may contain `:id`, replaced by the route's id (path-escaped). Only the listed query keys
// are forwarded; a POST body (JSON, 16 KB) is forwarded as is. The caller's role is checked by the router, not here.
func (h *AIHandler) Forward(method, upstream string, queryKeys ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "ai ops is disabled"})
			return
		}
		if h.BaseURL == "" || h.Signer == nil || !h.Signer.Keys.Enabled() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai service is not configured"})
			return
		}
		path := strings.ReplaceAll(upstream, ":id", url.PathEscape(c.Param("id")))
		q := url.Values{}
		for _, k := range queryKeys {
			if v := c.Query(k); v != "" {
				q.Set(k, v)
			}
		}
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
		var body io.Reader
		if method == http.MethodPost {
			raw, err := io.ReadAll(io.LimitReader(c.Request.Body, rcaMaxBody+1))
			if err != nil || len(raw) > rcaMaxBody {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
				return
			}
			body = bytes.NewReader(raw)
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if rid, ok := c.Get("requestID"); ok {
			req.Header.Set("X-Request-ID", fmt.Sprint(rid))
		}
		uid, _ := c.Get("userID")
		id, _ := uid.(uint64)
		pv, _ := c.Get("pwdVersion")
		pwdV, _ := pv.(int64)
		tok, jti, terr := h.Signer.IssueAgentToken(id, fmt.Sprint(c.Value("username")), fmt.Sprint(c.Value("userRole")), pwdV, 60*time.Second)
		if terr != nil {
			h.Logger.Error("AIHandler.Forward: cannot issue agent token", zap.Error(terr))
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai service is unavailable"})
			return
		}
		req.Header.Set("X-Agent-Token", tok)
		req.Header.Set("X-Agent-Run-ID", jti)
		resp, err := h.Client.Do(req)
		if err != nil {
			h.Logger.Warn("AIHandler.Forward: upstream unavailable", zap.Error(err))
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai service is unavailable"})
			return
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		c.Header("Cache-Control", "no-store")
		c.Data(resp.StatusCode, ct, out)
	}
}
