package handler

import (
	"api-gateway/internal/agent"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ProposalHandler serves POST /ai/proposals: the agent asks the gateway to sign "do this write on behalf of the user".
// The gateway checks the route is on the proposable list and returns a signed, 5-minute, single-use package that the UI
// shows to the user; confirming = the UI replays the request to the normal route with the header X-Agent-Proposal.
type ProposalHandler struct{ Signer *agent.Signer }

type proposalRequest struct {
	Method  string          `json:"method"`
	Path    string          `json:"path"`
	Body    json.RawMessage `json:"body"`
	Summary string          `json:"summary"`
}

func (h ProposalHandler) Create(c *gin.Context) {
	v, ok := c.Get("agentClaims")
	claims, _ := v.(*agent.Claims)
	if !ok || claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "agent token required"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 16<<10+1))
	var req proposalRequest
	if err != nil || len(raw) > 16<<10 || json.Unmarshal(raw, &req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proposal"})
		return
	}
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.Summary = strings.TrimSpace(req.Summary)
	if !agent.Proposable(req.Method, req.Path) || !agent.AllowsAction(claims.Role, req.Method, req.Path) {
		c.JSON(http.StatusForbidden, gin.H{"error": "this action cannot be proposed by the agent"})
		return
	}
	if req.Summary == "" || len(req.Summary) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "summary is required (max 200 characters)"})
		return
	}
	body := []byte(req.Body)
	if string(body) == "null" {
		body = nil
	}
	token, pc, err := h.Signer.SignProposal(claims.UserID(), claims.ID, req.Method, req.Path, agent.BodyHash(body), req.Summary, agent.ProposalTTL)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cannot sign the proposal"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"proposal": token, "id": pc.ID, "run_id": pc.RunID, "expires_at": pc.ExpiresAt.Time,
		"method": pc.Method, "path": pc.Path, "summary": pc.Summary,
	})
}
