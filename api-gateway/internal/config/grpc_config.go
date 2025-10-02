package config

import "api-gateway/pkg/clientname"

type GRPCAddrConfig map[string]string

// NewGRPCAddrConfig save address for client gRPC services
func NewGRPCAddrConfig() GRPCAddrConfig {
	return GRPCAddrConfig{
		clientname.AuthClientName:    "auth-service:50051",
		clientname.OrderClientName:   "order-service:50052",
		clientname.ProductClientName: "product-service:50053",
		clientname.UserClientName:    "user-service:50054",
	}
}
