package config

import "testing"

func TestNewGRPCAddrConfig(t *testing.T) {
	t.Setenv("AUTH_SERVICE_ADDR", "")
	t.Setenv("PRODUCT_SERVICE_ADDR", "")
	got := NewGRPCAddrConfig()
	if got["AuthClientAddr"] != "auth-service:50051" {
		t.Errorf("default AuthClientAddr = %q", got["AuthClientAddr"])
	}
	if got["ProductClientAddr"] != "product-service:50053" {
		t.Errorf("default ProductClientAddr = %q", got["ProductClientAddr"])
	}
	t.Setenv("AUTH_SERVICE_ADDR", "localhost:1")
	t.Setenv("PRODUCT_SERVICE_ADDR", "localhost:1")
	got = NewGRPCAddrConfig()
	if got["AuthClientAddr"] != "localhost:1" {
		t.Errorf("override AuthClientAddr = %q", got["AuthClientAddr"])
	}
	if got["ProductClientAddr"] != "localhost:1" {
		t.Errorf("override ProductClientAddr = %q", got["ProductClientAddr"])
	}
}
