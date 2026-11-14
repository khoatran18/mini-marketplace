package service

import "testing"

func TestSettingsRulesAreWholeUnitsFromTheEnvironment(t *testing.T) {
	t.Setenv("ORDER_PAYMENT_TTL_MIN", "20")
	t.Setenv("RETURN_WINDOW_DAYS", "10")
	t.Setenv("AUTO_DELIVER_DAYS", "0")
	t.Setenv("SHIPPING_FLAT_FEE", "25000")
	t.Setenv("FREE_SHIP_OVER", "0")
	got := SettingsFromEnv().Rules()
	want := ShopRules{PaymentTTLMinutes: 20, ReturnWindowDays: 10, AutoDeliverDays: 0, ShippingFlatFee: 25000, FreeShipOver: 0}
	if got != want {
		t.Errorf("Rules() = %+v, want %+v", got, want)
	}
}

func TestDefaultRulesMatchTheDocumentedDefaults(t *testing.T) {
	for _, k := range []string{"ORDER_PAYMENT_TTL_MIN", "RETURN_WINDOW_DAYS", "AUTO_DELIVER_DAYS", "SHIPPING_FLAT_FEE", "FREE_SHIP_OVER"} {
		t.Setenv(k, "")
	}
	r := SettingsFromEnv().Rules()
	if r.PaymentTTLMinutes != 15 || r.ReturnWindowDays != 7 || r.AutoDeliverDays != 7 || r.ShippingFlatFee != 30000 || r.FreeShipOver != 500000 {
		t.Errorf("defaults changed: %+v", r)
	}
}
