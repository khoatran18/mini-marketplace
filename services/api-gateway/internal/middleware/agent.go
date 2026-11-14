package middleware

import (
	"api-gateway/internal/agent"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"mini-marketplace/shared/reqctx"
)

// ProposalHeader carries the signed proposal when the UI replays an action the user confirmed.
const ProposalHeader = "X-Agent-Proposal"

// AgentAuth authenticates the AI agent with its own short-lived token (not a user JWT) and stores the claims under
// "agentClaims". It is attached per route (only POST /ai/proposals). It does not check pwd_version: a proposal does nothing
// until the user confirms it with their own, fully checked token.
func AgentAuth(signer *agent.Signer, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !signer.Keys.Enabled() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent actions are not configured"})
			c.Abort()
			return
		}
		tok, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}
		claims, err := signer.Verify(tok, agent.TypeAgent)
		if err != nil {
			logger.Warn("Middleware: invalid agent token")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}
		c.Set("agentClaims", claims)
		c.Next()
	}
}

// ConfirmProposal is attached after the normal auth (and role) middleware of a proposable write route. Without the
// proposal header the request is a direct user action and nothing changes. With it, the signed proposal must match this
// exact user, method, path and body, be unexpired and unused; then the run id is put in the request context so it travels
// to the service (gRPC metadata) and is stored with the action (agent_run_id). Any client-sent run id is never read.
// The action itself still runs with the user's real token and the route's own RBAC/ownership checks.
func ConfirmProposal(signer *agent.Signer, once agent.Once, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := c.GetHeader(ProposalHeader)
		if tok == "" {
			c.Next()
			return
		}
		deny := func(code int, msg string) {
			c.JSON(code, gin.H{"error": msg})
			c.Abort()
		}
		if !signer.Keys.Enabled() {
			deny(http.StatusServiceUnavailable, "agent actions are not configured")
			return
		}
		claims, err := signer.Verify(tok, agent.TypeProposal)
		if err != nil {
			deny(http.StatusUnauthorized, "Invalid or expired proposal")
			return
		}
		uid, _ := c.Get("userID")
		if id, ok := uid.(uint64); !ok || id != claims.UserID() {
			logger.Warn("Middleware: proposal belongs to another user", zap.String("proposal", claims.ID))
			deny(http.StatusForbidden, "Proposal belongs to another user")
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			deny(http.StatusBadRequest, "cannot read body")
			return
		}
		c.Request.Body = io.NopCloser(strings.NewReader(string(body))) // the handler reads it again
		if c.Request.Method != claims.Method || c.Request.URL.Path != claims.Path || agent.BodyHash(body) != claims.BodyHash {
			logger.Warn("Middleware: request differs from the signed proposal", zap.String("proposal", claims.ID))
			deny(http.StatusForbidden, "Request does not match the confirmed proposal")
			return
		}
		fresh, err := once.Take(c.Request.Context(), claims.ID, agent.ProposalTTL+agent.ProposalTTL/2)
		if err != nil {
			logger.Error("Middleware: cannot record proposal use", zap.Error(err))
			deny(http.StatusServiceUnavailable, "cannot verify the proposal right now, retry")
			return
		}
		if !fresh {
			deny(http.StatusConflict, "Proposal was already used")
			return
		}
		c.Set("agentRunID", claims.RunID)
		c.Request = c.Request.WithContext(reqctx.WithAgentRun(c.Request.Context(), claims.RunID))
		logger.Info("agent action confirmed by user",
			zap.String("agent_run_id", claims.RunID), zap.String("proposal", claims.ID),
			zap.Uint64("user_id", claims.UserID()), zap.String("method", claims.Method), zap.String("path", claims.Path))
		c.Next()
	}
}

// AgentOrUser replaces the user-auth middleware on the read routes listed in the scopes table. A request carrying a valid
// agent token (own key, aud gateway-agent, typ agent) is let through only if the route is listed for the token's role,
// and its identity (userID, role, username, pwdVersion) is filled from the token exactly as the JWT middleware would, so
// the route's own role check, ownership scoping and handlers run unchanged. The password version is checked like for a
// JWT, so a password change also stops running agents. Anything that is not an agent token goes to userAuth untouched,
// which rejects it if invalid. userAuth is the strict middleware, or OptionalAuth(strict) on public routes.
func AgentOrUser(signer *agent.Signer, scopes agent.Scopes, pwd *PwdVerifier, userAuth gin.HandlerFunc, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tok, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || !signer.Keys.Enabled() {
			userAuth(c)
			return
		}
		claims, err := signer.Verify(tok, agent.TypeAgent)
		if err != nil {
			userAuth(c) // not an agent token (or a bad one): the user path decides
			return
		}
		if !scopes.Allows(claims.Role, c.Request.Method, c.FullPath()) {
			logger.Warn("Middleware: agent token not allowed on this route",
				zap.String("agent_run_id", claims.ID), zap.String("role", claims.Role), zap.String("path", c.FullPath()))
			c.JSON(http.StatusForbidden, gin.H{"error": "the agent cannot call this route"})
			c.Abort()
			return
		}
		if pwd != nil {
			if err := pwd.Verify(c.Request.Context(), claims.UserID(), claims.PwdVersion); err != nil {
				if IsPwdChanged(err) {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired pwd version"})
				} else {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "cannot verify credentials right now, retry"})
				}
				c.Abort()
				return
			}
		}
		c.Set("userID", claims.UserID())
		c.Set("userRole", claims.Role)
		c.Set("username", claims.Username)
		c.Set("pwdVersion", claims.PwdVersion)
		c.Set("agentRunID", claims.ID)
		c.Request = c.Request.WithContext(reqctx.WithAgentRun(c.Request.Context(), claims.ID))
		c.Next()
	}
}
