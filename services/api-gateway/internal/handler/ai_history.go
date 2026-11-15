package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// History proxies the chat-history reads of ai-agent (list / one conversation, own or — admin only — everyone's).
// Identity is the same as /ai/chat: a short agent token minted from the verified JWT; the agent decides what that role may read
// (own conversations; /admin/conversations only for role admin). Only the whitelisted query parameters are forwarded.
func (h *AIHandler) History(upstreamPath string, byID bool, queryKeys ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.Enabled {
			c.JSON(http.StatusNotFound, gin.H{"error": "ai chat is disabled"})
			return
		}
		if h.BaseURL == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai service is not configured"})
			return
		}
		path := upstreamPath
		if byID {
			path += "/" + url.PathEscape(c.Param("id"))
		}
		q := url.Values{}
		for _, k := range queryKeys {
			if v := c.Query(k); v != "" {
				q.Set(k, v)
			}
		}
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.BaseURL+path, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-User-ID", fmt.Sprint(c.Value("userID")))
		req.Header.Set("X-User-Role", fmt.Sprint(c.Value("userRole")))
		if rid, ok := c.Get("requestID"); ok {
			req.Header.Set("X-Request-ID", fmt.Sprint(rid))
		}
		if h.Signer == nil || !h.Signer.Keys.Enabled() {
			req.Header.Set("Authorization", c.GetHeader("Authorization"))
		} else {
			uid, _ := c.Get("userID")
			id, _ := uid.(uint64)
			pv, _ := c.Get("pwdVersion")
			pwdV, _ := pv.(int64)
			tok, jti, terr := h.Signer.IssueAgentToken(id, fmt.Sprint(c.Value("username")), fmt.Sprint(c.Value("userRole")), pwdV, 60*time.Second)
			if terr != nil {
				h.Logger.Error("AIHandler.History: cannot issue agent token", zap.Error(terr))
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai service is unavailable"})
				return
			}
			req.Header.Set("X-Agent-Token", tok)
			req.Header.Set("X-Agent-Run-ID", jti)
		}
		resp, err := h.Client.Do(req)
		if err != nil {
			h.Logger.Warn("AIHandler.History: upstream unavailable", zap.Error(err))
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai service is unavailable"})
			return
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		c.Data(resp.StatusCode, ct, out)
	}
}
