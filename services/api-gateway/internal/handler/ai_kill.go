package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"api-gateway/internal/agent"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// paused answers 503 ai_paused when the AI kill switch is on (or cannot be read) and reports whether it did.
func (h *AIHandler) paused(c *gin.Context) bool {
	st, blocked := agent.Blocked(c.Request.Context(), h.Kill)
	if !blocked {
		return false
	}
	h.Logger.Warn("AIHandler: AI is switched off, request refused", zap.String("path", c.FullPath()), zap.String("by", st.By))
	c.Header("Retry-After", "60")
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai_paused", "message": "the AI assistant is temporarily switched off"})
	return true
}

// KillHandler serves the emergency switch (admin or operator, set by the router): GET shows it, PUT turns it on or off.
// Turning it on stops every agent path at once: new chats, the agent's tool calls, signing and confirming proposals, and the
// RCA watcher's system token; chats and investigations already running fail on their next call. People keep using the shop.
type KillHandler struct {
	Kill   agent.Kill
	Ops    *OpsHandler // where the audit row is written (ops_audit_log); nil = log only
	Logger *zap.Logger
}

const killReasonMax = 200

func (h KillHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.Kill == nil {
		c.JSON(http.StatusOK, gin.H{"paused": false, "available": false})
		return
	}
	st, err := h.Kill.State(c.Request.Context())
	if err != nil {
		// Redis unreachable: the agent is refused too (fail closed), so say it is paused
		c.JSON(http.StatusOK, gin.H{"paused": true, "available": true, "unknown": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"paused": st.On, "available": true, "by": st.By, "reason": st.Reason, "at": st.At})
}

func (h KillHandler) Put(c *gin.Context) {
	if h.Kill == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "the switch needs Redis"})
		return
	}
	var req struct {
		Paused *bool  `json:"paused"` // a pointer so that a missing field is an error, never a silent "switch off"
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Paused == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paused (true or false) is required"})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if utf8.RuneCountInString(reason) > killReasonMax {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("reason is at most %d characters", killReasonMax)})
		return
	}
	who := fmt.Sprintf("%v:%v", c.Value("userRole"), c.Value("userID"))
	var err error
	if *req.Paused {
		err = h.Kill.Set(c.Request.Context(), agent.KillState{By: who, Reason: reason, At: time.Now().UTC()})
	} else {
		err = h.Kill.Clear(c.Request.Context())
	}
	if err != nil {
		h.Logger.Error("KillHandler: cannot change the switch", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cannot change the switch right now"})
		return
	}
	h.Logger.Warn("AI kill switch changed", zap.Bool("paused", *req.Paused), zap.String("by", who), zap.String("reason", reason),
		zap.String("request_id", fmt.Sprint(c.Value("requestID"))))
	// The switch has already flipped: an emergency stop must not depend on ops-service being up. The audit row is written after,
	// and the answer says whether it was (the log line above is the fallback record).
	audited := true
	if err := h.Ops.Append(c, "/audit/events", map[string]any{"paused": *req.Paused, "reason": reason}); err != nil {
		audited = false
		h.Logger.Error("KillHandler: audit row not written to ops-service", zap.Error(err), zap.String("by", who), zap.Bool("paused", *req.Paused))
	}
	c.Header("Cache-Control", "no-store")
	st, _ := h.Kill.State(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"paused": st.On, "available": true, "by": st.By, "reason": st.Reason, "at": st.At, "audited": audited})
}
