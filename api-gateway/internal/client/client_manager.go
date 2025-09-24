package client

import (
	"api-gateway/internal/config"
	"api-gateway/pkg/clientname"
	"api-gateway/pkg/pb/authservice"
	orderpb "api-gateway/pkg/pb/orderservice"
	productpb "api-gateway/pkg/pb/productservice"
	userpb "api-gateway/pkg/pb/userservice"
	"errors"
	"fmt"
	"log"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ServiceClient is a client gRPC connection
type ServiceClient struct {
	Conn   *grpc.ClientConn
	Client any
}

// newServiceClient create client for each gRPC client
func newServiceClient[T any](addr string, createClient func(conn *grpc.ClientConn) T) (*ServiceClient, error) {

	// Test
	//interceptor := func(
	//	ctx context.Context,
	//	method string,
	//	req, reply interface{},
	//	cc *grpc.ClientConn,
	//	invoker grpc.UnaryInvoker,
	//	opts ...grpc.CallOption,
	//) error {
	//	fmt.Println("Calling method:", method, "on server:", cc.Target())
	//	return invoker(ctx, method, req, reply, cc, opts...)
	//}

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`),
		//grpc.WithUnaryInterceptor(interceptor),
	)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Create client successfully!\n")
	return &ServiceClient{
		Conn:   conn,
		Client: createClient(conn),
	}, nil
}

func (c *ServiceClient) closeService() error {
	return c.Conn.Close()
}

// ClientManager is responsible for Lazy Initialization Client, connecting with gRPC other services
type ClientManager struct {
	Clients            map[string]*ServiceClient
	AddrConfig         map[string]string
	ClientConstructors map[string]func(conn *grpc.ClientConn) any
	mu                 sync.Mutex
}

// NewClientManager create all client gRPC connections
func NewClientManager() *ClientManager {

	// Init client in ClientManager
	clients := make(map[string]*ServiceClient)

	// Load config addr
	addrCfg := config.NewGRPCAddrConfig()

	// Load constructors
	clientConstructors := newClientConstructors()

	// Return ClientManager
	return &ClientManager{
		Clients:            clients,
		AddrConfig:         addrCfg,
		ClientConstructors: clientConstructors,
	}
}

func newClientConstructors() map[string]func(conn *grpc.ClientConn) any {

	authConstructor := func(conn *grpc.ClientConn) any {
		return authpb.NewAuthServiceClient(conn)
	}
	orderConstructor := func(conn *grpc.ClientConn) any {
		return orderpb.NewOrderServiceClient(conn)
	}
	productConstructor := func(conn *grpc.ClientConn) any {
		return productpb.NewProductServiceClient(conn)
	}
	userConstructor := func(conn *grpc.ClientConn) any {
		return userpb.NewUserServiceClient(conn)
	}

	constructors := map[string]func(conn *grpc.ClientConn) any{
		clientname.AuthClientName:    authConstructor,
		clientname.OrderClientName:   orderConstructor,
		clientname.ProductClientName: productConstructor,
		clientname.UserClientName:    userConstructor,
	}

	return constructors
}

// CloseAll close all client gRPC connections
func (cm *ClientManager) CloseAll() {
	clients := cm.Clients
	for name, client := range clients {
		if client != nil {
			err := client.closeService()
			if err != nil {
				log.Printf("Close client client failed: %v", name)
			}
			log.Printf("Close client client success: %v", name)
		} else {
			log.Printf("Client is nil: %v", name)
		}
	}
}

// GetOrCreateServiceClient is responsible for getting client if existed else creating new client
func (cm *ClientManager) GetOrCreateServiceClient(serviceName string) (any, error) {

	// Lock to avoid multi-initialization
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if client existed
	if serviceClient, ok := cm.Clients[serviceName]; ok {
		return serviceClient.Client, nil
	}

	// Create Client
	addr := cm.AddrConfig[serviceName]
	if addr == "" {
		return nil, errors.New(fmt.Sprintf("addr for %v (GRPCConfig) is empty", serviceName))
	}

	newClient, err := newServiceClient(addr, cm.ClientConstructors[serviceName])
	if err != nil {
		return nil, fmt.Errorf("AuthClient not existed but init failed %v", err)
	}

	cm.Clients[serviceName] = newClient
	return newClient.Client, nil
}

// GetOrCreateAuthClient is responsible for getting AuthClient if existed else creating AuthClient
//func (cm *ClientManager) GetOrCreateAuthClient() (authpb.AuthServiceClient, error) {
//
//	// Lock to avoid multi-initialization
//	cm.mu.Lock()
//	defer cm.mu.Unlock()
//
//	// Check if AuthClient existed
//	if cm.Clients[AuthClient] != nil {
//		client, ok := cm.Clients["AuthClient"].Client.(authpb.AuthServiceClient)
//		if !ok {
//			return nil, errors.New("AuthClient existed but is not valid")
//		}
//		return client, nil
//	}
//
//	// Create AuthClient
//	addr := cm.AddrConfig["AuthClientAddr"]
//	if addr == "" {
//		return nil, errors.New("addr for AuthClient (GRPCConfig) is empty")
//	}
//
//	authClient, err := newServiceClient(addr, func(conn *grpc.ClientConn) interface{} {
//		return authpb.NewAuthServiceClient(conn)
//	})
//	if err != nil {
//		return nil, fmt.Errorf("AuthClient not existed but init failed %v", err)
//	}
//
//	cm.Clients["AuthClient"] = authClient
//	return authClient.Client.(authpb.AuthServiceClient), nil
//}
//
//// GetOrCreateProductClient is responsible for getting AuthClient if existed else creating ProductClient
//func (cm *ClientManager) GetOrCreateProductClient() (productpb.ProductServiceClient, error) {
//
//	// Lock to avoid multi-initialization
//	cm.mu.Lock()
//	defer cm.mu.Unlock()
//
//	// Check if AuthClient existed
//	if cm.Clients[ProductClient] != nil {
//		client, ok := cm.Clients["ProductClient"].Client.(productpb.ProductServiceClient)
//		if !ok {
//			return nil, errors.New("ProductClient existed but is not a client for ProductService")
//		}
//		return client, nil
//	}
//
//	// Create ProductClient
//	addr := cm.AddrConfig["ProductClientAddr"]
//	if addr == "" {
//		return nil, errors.New("ProductClientAddr (GRPCConfig) is empty")
//	}
//
//	productClient, err := newServiceClient(addr, func(conn *grpc.ClientConn) interface{} {
//		return productpb.NewProductServiceClient(conn)
//	})
//	if err != nil {
//		return nil, fmt.Errorf("ProductClient not existed but init failed %v", err)
//	}
//
//	cm.Clients["ProductClient"] = productClient
//	return productClient.Client.(productpb.ProductServiceClient), nil
//}
