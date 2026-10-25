package dto

type ErrorResponse struct {
	Error string `json:"error" example:"invalid request"`
	// RequestID khớp header X-Request-ID để tra log; bỏ trống khi không có (additive, không phá contract).
	RequestID string `json:"request_id,omitempty" example:"9f1c2d3e-0000-4000-8000-123456789abc"`
}
