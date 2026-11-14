package handler

import (
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
// the caller and forwards identity; model logic lives upstream. The agent calls the normal REST API with the caller's own JWT
// (forwarded as Authorization), so the existing RBAC/ownership checks still apply to every tool call.
type AIHandler struct {
	Enabled     bool
	BaseURL     string
	MaxDuration time.Duration // cap for one stream (client disconnect cancels earlier)
	Client      *http.Client  // no overall Timeout: a stream outlives any fixed request timeout
	Logger      *zap.Logger
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
	req.Header.Set("Authorization", c.GetHeader("Authorization"))
	req.Header.Set("X-User-ID", fmt.Sprint(c.Value("userID")))
	req.Header.Set("X-User-Role", fmt.Sprint(c.Value("userRole")))
	if rid, ok := c.Get("requestID"); ok {
		req.Header.Set("X-Request-ID", fmt.Sprint(rid))
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
