package config

type GRPCAddrConfig map[string]string

func NewGRPCAddrConfig() GRPCAddrConfig {
	return GRPCAddrConfig{
		"AuthClientAddr": "localhost:50051",
	}
}
