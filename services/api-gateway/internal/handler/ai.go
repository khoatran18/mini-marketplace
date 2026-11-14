package handler

import (
	"api-gateway/internal/agent"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// FeaturesHandler serves GET /config/features: which optional capabilities are on, so the UI can hide the AI entry points and
// fall back to the normal (non-AI) flow. Public, no secrets, never cached.
type FeaturesHandler struct{ AIChat, AIOps bool }

func (h FeaturesHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"features": gin.H{"ai_chat": h.AIChat, "ai_ops": h.AIOps}})
}

// AIHandler streams POST /ai/chat to the agent service (Server-Sent Events) without buffering. The gateway only authenticates
// the caller and forwards identity; model logic lives upstream. The agent calls the normal REST API with its own agent token (ADR-42; the
// user's JWT is forwarded only when no agent keys are set), so the existing RBAC/ownership checks still apply to every tool call.
type AIHandler struct {
	Enabled     bool
	BaseURL     string
	MaxDuration time.Duration // cap for one stream (client disconnect cancels earlier)
	Client      *http.Client  // no overall Timeout: a stream outlives any fixed request timeout
	Logger      *zap.Logger
	Signer      *agent.Signer // ADR-42: issues the short-lived agent token sent in X-Agent-Token (nil/no keys = not sent)
}

// NewAIHandler builds the handler. Disabled (flag off) answers 404; enabled without an upstream URL answers 503.
func NewAIHandler(enabled bool, baseURL string, maxDuration time.Duration, logger *zap.Logger) *AIHandler {
	if maxDuration <= 0 {
		maxDuration = 120 * time.Second
	}
	return &AIHandler{
		Enabled: enabled, BaseURL: strings.TrimRight(baseURL, "/"), MaxDuration: maxDuration, Logger: logger,
		Client: &http.Client{Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment, MaxIdleConns: 20, IdleConnTimeout: 60 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second, // the model must start answering (first byte of headers) within 30 s
		}},
	}
}

const aiMaxBody = 64 << 10

func (h *AIHandler) Chat(c *gin.Context) {
	if !h.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "ai chat is disabled"})
		return
	}
	if h.BaseURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai service is not configured"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, aiMaxBody+1))
	if err != nil || len(body) > aiMaxBody {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.MaxDuration)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.BaseURL+"/chat", bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	// Identity comes only from the verified JWT (set by the auth middleware), never from the request.
	req.Header.Set("X-User-ID", fmt.Sprint(c.Value("userID")))
	req.Header.Set("X-User-Role", fmt.Sprint(c.Value("userRole")))
	if rid, ok := c.Get("requestID"); ok {
		req.Header.Set("X-Request-ID", fmt.Sprint(rid))
	}
	// ADR-42: a token made for the agent (own key, ~150 s, bound to this user/role, jti = run id). When it is issued the
	// user's JWT is NOT forwarded: the agent reads only the routes in agent/scopes.go with this token. Without agent keys the
	// old behaviour stays (the user's Authorization is forwarded) so a deployment without keys keeps working.
	if h.Signer == nil || !h.Signer.Keys.Enabled() {
		req.Header.Set("Authorization", c.GetHeader("Authorization"))
	} else {
		uid, _ := c.Get("userID")
		id, _ := uid.(uint64)
		pv, _ := c.Get("pwdVersion")
		pwdV, _ := pv.(int64)
		tok, jti, terr := h.Signer.IssueAgentToken(id, fmt.Sprint(c.Value("username")), fmt.Sprint(c.Value("userRole")), pwdV, h.MaxDuration+30*time.Second)
		if terr != nil {
			h.Logger.Error("AIHandler: cannot issue agent token", zap.Error(terr))
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai service is unavailable"})
			return
		}
		req.Header.Set("X-Agent-Token", tok)
		req.Header.Set("X-Agent-Run-ID", jti)
		h.Logger.Info("agent run started", zap.String("agent_run_id", jti), zap.Uint64("user_id", id), zap.String("request_id", fmt.Sprint(c.Value("requestID"))))
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		h.Logger.Warn("AIHandler: upstream unavailable", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai service is unavailable"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		out, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		c.Data(resp.StatusCode, ct, out)
		return
	}

	// Stream: headers first, then every chunk is flushed immediately. The server write deadline is lifted for this response only.
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
	w := c.Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no") // proxies that honour it (nginx) must not buffer
	w.WriteHeader(http.StatusOK)
	w.Flush()
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return // client went away: cancel() stops the upstream call
			}
			w.Flush()
		}
		if rerr != nil {
			if !errors.Is(rerr, io.EOF) && ctx.Err() == nil {
				h.Logger.Warn("AIHandler: stream broke", zap.Error(rerr))
				_, _ = w.WriteString("event: error\ndata: {\"error\":\"stream interrupted\"}\n\n")
			} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				_, _ = w.WriteString("event: error\ndata: {\"error\":\"stream time limit reached\"}\n\n")
			}
			w.Flush()
			return
		}
	}
}
