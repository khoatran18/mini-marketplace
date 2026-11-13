package handler

import (
	"api-gateway/pkg/dto"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Admin account management. The route group already restricts the caller to role admin; the actor id comes from the token.

// ListAccounts GET /admin/accounts?role=&q=&limit=
func (h *AuthHandler) ListAccounts(c *gin.Context) {
	limit, _ := strconv.ParseUint(c.Query("limit"), 10, 32)
	res, err := h.Service.ListAccounts(c.Request.Context(), c.Query("role"), c.Query("q"), uint32(limit))
	if err != nil {
		respondError(c, h.Logger, "AuthHandler ListAccounts warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// CreateStaffAccount POST /admin/accounts (admin or dev accounts only)
func (h *AuthHandler) CreateStaffAccount(c *gin.Context) {
	var req dto.CreateStaffInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	if len(req.Password) < 8 {
		badRequest(c, "Password must be at least 8 characters")
		return
	}
	res, err := h.Service.CreateStaffAccount(c.Request.Context(), &req)
	if err != nil {
		respondError(c, h.Logger, "AuthHandler CreateStaffAccount warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *AuthHandler) staffIDs(c *gin.Context) (id, actor uint64, ok bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	actor, hasActor := currentUserID(c)
	if err != nil || id == 0 || !hasActor {
		badRequest(c, "invalid account id")
		return 0, 0, false
	}
	return id, actor, true
}

// DeleteStaffAccount DELETE /admin/accounts/:id
func (h *AuthHandler) DeleteStaffAccount(c *gin.Context) {
	id, actor, ok := h.staffIDs(c)
	if !ok {
		return
	}
	res, err := h.Service.DeleteStaffAccount(c.Request.Context(), id, actor)
	if err != nil {
		respondError(c, h.Logger, "AuthHandler DeleteStaffAccount warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ResetStaffPassword POST /admin/accounts/:id/reset-password
func (h *AuthHandler) ResetStaffPassword(c *gin.Context) {
	id, actor, ok := h.staffIDs(c)
	if !ok {
		return
	}
	var req dto.ResetStaffPasswordInput
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, GetErrorString(err.Error()))
		return
	}
	if len(req.NewPassword) < 8 {
		badRequest(c, "Password must be at least 8 characters")
		return
	}
	res, err := h.Service.ResetStaffPassword(c.Request.Context(), id, actor, req.NewPassword)
	if err != nil {
		respondError(c, h.Logger, "AuthHandler ResetStaffPassword warn", err)
		return
	}
	c.JSON(http.StatusOK, res)
}
