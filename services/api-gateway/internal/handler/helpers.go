package handler

import (
	"api-gateway/pkg/dto"
	"buf.build/go/protovalidate"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// currentUserID returns the authenticated user ID set by AuthMiddleware.
func currentUserID(c *gin.Context) (uint64, bool) {
	v, exists := c.Get("userID")
	if !exists {
		return 0, false
	}
	id, ok := v.(uint64)
	return id, ok
}

// currentUser returns the authenticated username and role set by AuthMiddleware.
func currentUser(c *gin.Context) (username, role string, ok bool) {
	u, ok1 := c.Get("username")
	r, ok2 := c.Get("userRole")
	username, ok3 := u.(string)
	role, ok4 := r.(string)
	return username, role, ok1 && ok2 && ok3 && ok4
}

// httpStatusFromError maps a gRPC status error to an HTTP status code.
func httpStatusFromError(err error) int {
	// Requests rejected by the protobuf validation rules are client errors
	var validationErr *protovalidate.ValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest
	}
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.AlreadyExists, codes.Aborted:
		return http.StatusConflict
	case codes.FailedPrecondition:
		return http.StatusUnprocessableEntity
	case codes.Unavailable, codes.DeadlineExceeded:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// respondError logs err and writes a JSON error with a status derived from the gRPC code.
func respondError(c *gin.Context, logger *zap.Logger, msg string, err error) {
	logger.Warn(msg, zap.Error(err))
	c.JSON(httpStatusFromError(err), dto.ErrorResponse{Error: GetErrorString(err.Error())})
}

func badRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: msg})
}
