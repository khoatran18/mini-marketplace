package config

import "api-gateway/pkg/clientname"

type GRPCAddrConfig map[string]string

// NewGRPCAddrConfig save address for client gRPC services
func NewGRPCAddrConfig() GRPCAddrConfig {
	return GRPCAddrConfig{
		clientname.AuthClientName:    "localhost:50051",
		clientname.OrderClientName:   "localhost:50052",
		clientname.ProductClientName: "localhost:50053",
		clientname.UserClientName:    "localhost:50054",
	}
}
