package config

import "os"

type GRPCAddrConfig map[string]string

// grpcAddr returns the env value of key, or def when it is unset/empty.
func grpcAddr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// NewGRPCAddrConfig save address for client gRPC services (overridable via *_SERVICE_ADDR env)
func NewGRPCAddrConfig() GRPCAddrConfig {
	return GRPCAddrConfig{
		"AuthClientAddr":    grpcAddr("AUTH_SERVICE_ADDR", "auth-service:50051"),
		"ProductClientAddr": grpcAddr("PRODUCT_SERVICE_ADDR", "product-service:50053"),
	}
}
