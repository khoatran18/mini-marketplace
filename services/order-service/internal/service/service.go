package service

import (
	"context"
	"order-service/internal/client/productclient"
	"order-service/internal/client/serviceclientmanager"
	"order-service/internal/config/messagequeue"
	"order-service/internal/config/messagequeue/kafkaimpl"
	"order-service/internal/repository"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Settings are the tunable business rules (environment variables, see deploy/.env.example).
type Settings struct {
	PaymentTTL        time.Duration // ORDER_PAYMENT_TTL_MIN  (default 15): unpaid orders expire and release their stock
	ReturnWindow      time.Duration // RETURN_WINDOW_DAYS     (default 7)
	AutoDeliverAfter  time.Duration // AUTO_DELIVER_DAYS      (default 7, 0 = off): shipped orders become delivered
	ShippingFlatMinor int64         // SHIPPING_FLAT_FEE      (default 30000 currency units)
	FreeShipOverMinor int64         // FREE_SHIP_OVER         (default 500000, 0 = never free)
}

// SettingsFromEnv reads the settings with their defaults.
func SettingsFromEnv() Settings {
	return Settings{
		PaymentTTL:        time.Duration(envInt("ORDER_PAYMENT_TTL_MIN", 15)) * time.Minute,
		ReturnWindow:      time.Duration(envInt("RETURN_WINDOW_DAYS", 7)) * 24 * time.Hour,
		AutoDeliverAfter:  time.Duration(envInt("AUTO_DELIVER_DAYS", 7)) * 24 * time.Hour,
		ShippingFlatMinor: int64(envInt("SHIPPING_FLAT_FEE", 30000)) * 100,
		FreeShipOverMinor: int64(envInt("FREE_SHIP_OVER", 500000)) * 100,
	}
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v >= 0 {
		return v
	}
	return def
}

// OrderService holds the order business logic.
type OrderService struct {
	OrderRepo   *repository.OrderRepository
	ZapLogger   *zap.Logger
	MQProducer  messagequeue.Producer
	MQConsumer  messagequeue.Consumer
	KafkaClient *kafkaimpl.KafkaClient
	SCM         *serviceclientmanager.ServiceClientManager
	Settings    Settings
}

// NewOrderService creates the service with settings from the environment.
func NewOrderService(repo *repository.OrderRepository, logger *zap.Logger, scm *serviceclientmanager.ServiceClientManager,
	producer messagequeue.Producer, consumer messagequeue.Consumer, kafkaClient *kafkaimpl.KafkaClient) *OrderService {
	return &OrderService{
		OrderRepo: repo, ZapLogger: logger, SCM: scm, MQProducer: producer, MQConsumer: consumer, KafkaClient: kafkaClient,
		Settings: SettingsFromEnv(),
	}
}

// productActive is the product-service status of products that can be ordered.
const productActive = "active"

// maxItemQuantity guards against absurd quantities (and integer overflow when pricing).
const maxItemQuantity = 100000

// maxCheckoutLines bounds the number of different products in one checkout.
const maxCheckoutLines = 50

// loadCatalog fetches the products by id; missing ids are simply absent from the result.
func (s *OrderService) loadCatalog(ctx context.Context, ids []uint64) (map[uint64]*productclient.ProductDTOClient, error) {
	out := map[uint64]*productclient.ProductDTOClient{}
	if len(ids) == 0 {
		return out, nil
	}
	res, err := s.SCM.ProductServiceClient.GetProductsByID(ctx, &productclient.GetProductsByIDInput{IDs: ids})
	if err != nil {
		s.ZapLogger.Error("OrderService: can not load products", zap.Error(err))
		return nil, status.Error(codes.Unavailable, "product service is unavailable")
	}
	for _, p := range res.Products {
		out[p.ID] = p
	}
	return out, nil
}

func (s *OrderService) shippingFeeMinor(subtotalMinor int64) int64 {
	if s.Settings.FreeShipOverMinor > 0 && subtotalMinor >= s.Settings.FreeShipOverMinor {
		return 0
	}
	return s.Settings.ShippingFlatMinor
}
