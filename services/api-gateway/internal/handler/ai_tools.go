package handler

import (
	"api-gateway/internal/agent"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ToolsHandler serves GET /ai/tools: the tools of the agent token's role, straight from the scopes table the gateway itself
// enforces (one source of truth). ai-agent builds its tool schemas from this instead of keeping its own list.
type ToolsHandler struct{ Scopes agent.Scopes }

func (h ToolsHandler) List(c *gin.Context) {
	v, _ := c.Get("agentClaims")
	claims, ok := v.(*agent.Claims)
	if !ok || claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "agent token required"})
		return
	}
	tools := h.Scopes.Tools(claims.Role)
	if tools == nil {
		tools = []agent.Tool{}
	}
	c.Header("Cache-Control", "no-store")
	actions := agent.ActionsFor(claims.Role)
	if actions == nil {
		actions = []agent.Action{}
	}
	c.JSON(http.StatusOK, gin.H{"role": claims.Role, "tools": tools, "actions": actions})
}
