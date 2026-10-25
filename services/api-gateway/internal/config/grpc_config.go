package config

import (
	"os"

	"api-gateway/pkg/clientname"
)

type GRPCAddrConfig map[string]string

// grpcAddr returns the env value of key, or def when it is unset/empty.
func grpcAddr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// NewGRPCAddrConfig save address for client gRPC services
// (override with AUTH_SERVICE_ADDR, ORDER_SERVICE_ADDR, PRODUCT_SERVICE_ADDR, USER_SERVICE_ADDR,
// PAYMENT_SERVICE_ADDR, ANALYTICS_SERVICE_ADDR).
func NewGRPCAddrConfig() GRPCAddrConfig {
	return GRPCAddrConfig{
		clientname.AuthClientName:      grpcAddr("AUTH_SERVICE_ADDR", "auth-service:50051"),
		clientname.OrderClientName:     grpcAddr("ORDER_SERVICE_ADDR", "order-service:50052"),
		clientname.ProductClientName:   grpcAddr("PRODUCT_SERVICE_ADDR", "product-service:50053"),
		clientname.UserClientName:      grpcAddr("USER_SERVICE_ADDR", "user-service:50054"),
		clientname.PaymentClientName:   grpcAddr("PAYMENT_SERVICE_ADDR", "payment-service:50057"),
		clientname.AnalyticsClientName: grpcAddr("ANALYTICS_SERVICE_ADDR", "analytics-service:50055"),
	}
}
