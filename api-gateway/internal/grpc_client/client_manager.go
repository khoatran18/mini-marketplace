package grpc_client

import (
	"api-gateway/internal/config"
	pb "api-gateway/pkg/pb/auth_service"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ServiceClient is a client gRPC connection
type ServiceClient struct {
	Conn   *grpc.ClientConn
	Client any
}

func NewServiceClient[T any](addr string, createClient func(conn *grpc.ClientConn) T) (*ServiceClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &ServiceClient{
		Conn:   conn,
		Client: createClient(conn),
	}, nil
}

func (c *ServiceClient) CloseService() error {
	return c.Conn.Close()
}

// ClientManager is responsible for connecting with gRPC other services
type ClientManager struct {
	Clients map[string]*ServiceClient
}

// NewClientManager create all client gRPC connections
func NewClientManager() *ClientManager {

	// Init client in ClientManager
	clients := make(map[string]*ServiceClient)
	// Load config addr
	addrCfg := config.NewGRPCAddrConfig()

	// Load all service
	authClient, err := NewServiceClient(addrCfg["AuthClientAddr"], func(conn *grpc.ClientConn) interface{} {
		return pb.NewAuthServiceClient(conn)
	})
	if err != nil {
		log.Printf("Create AuthClient failed: %v", err)
	}
	clients["AuthClient"] = authClient

	// Return ClientManager
	return &ClientManager{
		Clients: clients,
	}
}

// CloseAll close all client gRPC connections
func (cm *ClientManager) CloseAll() {
	clients := cm.Clients
	for name, client := range clients {
		if client != nil {
			err := client.CloseService()
			if err != nil {
				log.Printf("Close service client failed: %v", name)
			}
			log.Printf("Close service client success: %v", name)
		}
	}
}
