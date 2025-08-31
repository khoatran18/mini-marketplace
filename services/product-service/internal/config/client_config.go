package config

type GRPCAddrConfig map[string]string

// NewGRPCAddrConfig save address for client gRPC services
func NewGRPCAddrConfig() GRPCAddrConfig {
	return GRPCAddrConfig{
		"AuthClientAddr": "localhost:50051",
	}
}
