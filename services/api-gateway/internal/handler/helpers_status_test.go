package handler

import (
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestHTTPStatusDeadlineAndUnavailable(t *testing.T) {
	if got := httpStatusFromError(status.Error(codes.DeadlineExceeded, "x")); got != http.StatusGatewayTimeout {
		t.Fatalf("DeadlineExceeded -> %d", got)
	}
	if got := httpStatusFromError(status.Error(codes.Unavailable, "x")); got != http.StatusServiceUnavailable {
		t.Fatalf("Unavailable -> %d", got)
	}
}
