package handler

import (
	"api-gateway/internal/client"
	"api-gateway/internal/client/authclient"
	"api-gateway/internal/client/orderclient"
	"api-gateway/internal/client/productclient"
	"api-gateway/internal/client/userclient"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ManagerHandler save handlers for all client gRPC
type ManagerHandler struct {
	AuthHandler    *AuthHandler
	OrderHandler   *OrderHandler
	ProductHandler *ProductHandler
	UserHandler    *UserHandler
}

// NewHandlerManager init handlers for ManagerHandler
func NewHandlerManager(cm *client.ClientManager, logger *zap.Logger) *ManagerHandler {

	// Create AuthService (wrap AuthClient)
	authService := authclient.NewAuthClient(nil, cm, logger) // AuthClient is nil until it is called
	authHandler := NewAuthHandler(authService, logger)

	// Create OrderService (wrap OrderClient)
	orderService := orderclient.NewOrderClient(nil, cm, logger)
	orderHandler := NewOrderHandler(orderService, logger)

	// Create ProductService (wrap ProductClient)
	productService := productclient.NewProductClient(nil, cm, logger)
	productHandler := NewProductHandler(productService, logger)

	// Create UserService (wrap UserClient)
	userService := userclient.NewUserClient(nil, cm, logger)
	userHandler := NewUserHandler(userService, logger)

	// Return ManagerHandler
	return &ManagerHandler{
		AuthHandler:    authHandler,
		OrderHandler:   orderHandler,
		ProductHandler: productHandler,
		UserHandler:    userHandler,
	}
}

func GetErrorString(s string) string {
	s = strings.TrimSpace(s)
	lastColon := strings.LastIndex(s, ":")
	lastEqual := strings.LastIndex(s, "=")

	last := lastColon
	if lastEqual > lastColon {
		last = lastEqual
	}

	var str string
	if last == -1 {
		str = s
	} else if last+1 < len(s) {
		str = strings.TrimSpace(s[last+1:])
	} else {
		str = ""
	}

	if str == "" {
		return ""
	}

	return strings.ToUpper(string(str[0])) + str[1:]
}

func getQueryInt(c *gin.Context, key string, defaultVal int) (int, error) {
	valStr := c.Query(key)
	if valStr == "" {
		return defaultVal, nil
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, err
	}
	return val, nil
}
