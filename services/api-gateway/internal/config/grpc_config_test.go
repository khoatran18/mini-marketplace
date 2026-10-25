package config

import (
	"testing"

	"api-gateway/pkg/clientname"
)

func TestNewGRPCAddrConfigDefaults(t *testing.T) {
	for _, k := range []string{"AUTH_SERVICE_ADDR", "ORDER_SERVICE_ADDR", "PRODUCT_SERVICE_ADDR", "USER_SERVICE_ADDR", "PAYMENT_SERVICE_ADDR", "ANALYTICS_SERVICE_ADDR"} {
		t.Setenv(k, "")
	}
	want := map[string]string{
		clientname.AuthClientName:      "auth-service:50051",
		clientname.OrderClientName:     "order-service:50052",
		clientname.ProductClientName:   "product-service:50053",
		clientname.UserClientName:      "user-service:50054",
		clientname.PaymentClientName:   "payment-service:50057",
		clientname.AnalyticsClientName: "analytics-service:50055",
	}
	got := NewGRPCAddrConfig()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestNewGRPCAddrConfigEnvOverride(t *testing.T) {
	t.Setenv("AUTH_SERVICE_ADDR", "localhost:6001")
	t.Setenv("ORDER_SERVICE_ADDR", "localhost:6002")
	t.Setenv("PRODUCT_SERVICE_ADDR", "localhost:6003")
	t.Setenv("USER_SERVICE_ADDR", "localhost:6004")
	t.Setenv("PAYMENT_SERVICE_ADDR", "localhost:6007")
	t.Setenv("ANALYTICS_SERVICE_ADDR", "localhost:6005")
	got := NewGRPCAddrConfig()
	want := map[string]string{
		clientname.AuthClientName:      "localhost:6001",
		clientname.OrderClientName:     "localhost:6002",
		clientname.ProductClientName:   "localhost:6003",
		clientname.UserClientName:      "localhost:6004",
		clientname.PaymentClientName:   "localhost:6007",
		clientname.AnalyticsClientName: "localhost:6005",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}
