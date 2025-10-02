package config

type GRPCAddrConfig map[string]string

// NewGRPCAddrConfig save address for client gRPC services
func NewGRPCAddrConfig() GRPCAddrConfig {
	return GRPCAddrConfig{
		"AuthClientAddr":    "auth-client:50051",
		"ProductClientAddr": "product-client:50053",
	}
}
