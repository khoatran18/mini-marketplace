package clientmanager

import (
	"errors"
	"log"
	"user-service/internal/config"
	authpb "user-service/pkg/client/authclient"
	productpb "user-service/pkg/client/productclient"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ServiceClient is a client gRPC connection
type ServiceClient struct {
	Conn   *grpc.ClientConn
	Client any
}

// NewServiceClient create client for each gRPC client
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

// ClientManager is responsible for Lazy Initialization Client, connecting with gRPC other services
type ClientManager struct {
	Clients    map[string]*ServiceClient
	AddrConfig map[string]string
}

// NewClientManager create all client gRPC connections
func NewClientManager() *ClientManager {

	// Init client in ClientManager
	clients := make(map[string]*ServiceClient)

	// Load config addr
	addrCfg := config.NewGRPCAddrConfig()

	// Return ClientManager
	return &ClientManager{
		Clients:    clients,
		AddrConfig: addrCfg,
	}
}

// CloseAll close all client gRPC connections
func (cm *ClientManager) CloseAll() {
	clients := cm.Clients
	for name, client := range clients {
		if client != nil {
			err := client.CloseService()
			if err != nil {
				log.Printf("Close client client failed: %v", name)
			}
			log.Printf("Close client client success: %v", name)
		}
		log.Printf("Client is nil: %v", name)
	}
}

// GetOrCreateAuthClient is responsible for getting AuthClient if existed else creating AuthClient
func (cm *ClientManager) GetOrCreateAuthClient() (authpb.AuthServiceClient, error) {

	// Check if AuthClient existed
	if cm.Clients["AuthClient"] != nil {
		client, ok := cm.Clients["AuthClient"].Client.(authpb.AuthServiceClient)
		if !ok {
			return nil, errors.New("AuthClient existed but is not a client for AuthService")
		}
		return client, nil
	}

	// Create AuthClient
	addr := cm.AddrConfig["AuthClientAddr"]
	if addr == "" {
		return nil, errors.New("AuthClientAddr (GRPCConfig) is empty")
	}

	authClient, err := NewServiceClient(addr, func(conn *grpc.ClientConn) interface{} {
		return authpb.NewAuthServiceClient(conn)
	})
	if err != nil {
		return nil, errors.New("AuthClient not existed but init failed")
	}

	cm.Clients["AuthClient"] = authClient
	return authClient.Client.(authpb.AuthServiceClient), nil
}

// GetOrCreateProductClient is responsible for getting AuthClient if existed else creating AuthClient
func (cm *ClientManager) GetOrCreateProductClient() (productpb.ProductServiceClient, error) {

	// Check if AuthClient existed
	if cm.Clients["ProductClient"] != nil {
		client, ok := cm.Clients["ProductClient"].Client.(productpb.ProductServiceClient)
		if !ok {
			return nil, errors.New("ProductClient existed but is not a client for AuthService")
		}
		return client, nil
	}

	// Create AuthClient
	addr := cm.AddrConfig["ProductClientAddr"]
	if addr == "" {
		return nil, errors.New("ProductClientAddr (GRPCConfig) is empty")
	}

	productClient, err := NewServiceClient(addr, func(conn *grpc.ClientConn) interface{} {
		return productpb.NewProductServiceClient(conn)
	})
	if err != nil {
		return nil, errors.New("ProductClient not existed but init failed")
	}

	cm.Clients["ProductClient"] = productClient
	return productClient.Client.(productpb.ProductServiceClient), nil
}
