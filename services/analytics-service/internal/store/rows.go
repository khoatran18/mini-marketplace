package store

// Row types: json tags = ClickHouse column names (INSERT ... FORMAT JSONEachRow). Time columns are strings in
// the format produced by Time().

type EventRow struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	Ts               string `json:"ts"`
	TsServer         string `json:"ts_server"`
	AnonymousID      string `json:"anonymous_id"`
	SessionID        string `json:"session_id"`
	UserID           uint64 `json:"user_id"`
	Role             string `json:"role"`
	StoreID          uint64 `json:"store_id"`
	Surface          string `json:"surface"`
	Path             string `json:"path"`
	Referrer         string `json:"referrer"`
	ProductID        uint64 `json:"product_id"`
	Position         uint16 `json:"position"`
	Props            string `json:"props"`
	DeviceType       string `json:"device_type"`
	OS               string `json:"os"`
	Country          string `json:"country"`
	UAFamily         string `json:"ua_family"`
	AnalyticsConsent uint8  `json:"analytics_consent"`
	IPHash           string `json:"ip_hash"`
	AppVersion       string `json:"app_version"`
}

type OrderStatusRow struct {
	OrderID          uint64 `json:"order_id"`
	Status           string `json:"status"`
	CheckoutID       string `json:"checkout_id"`
	BuyerID          uint64 `json:"buyer_id"`
	StoreID          uint64 `json:"store_id"`
	PaymentMethod    string `json:"payment_method"`
	PaymentStatus    string `json:"payment_status"`
	SubtotalMinor    int64  `json:"subtotal_minor"`
	ShippingFeeMinor int64  `json:"shipping_fee_minor"`
	TotalMinor       int64  `json:"total_minor"`
	At               string `json:"at"`
}

type OrderItemRow struct {
	OrderID        uint64 `json:"order_id"`
	ItemID         uint64 `json:"item_id"`
	StoreID        uint64 `json:"store_id"`
	ProductID      uint64 `json:"product_id"`
	CategoryID     uint64 `json:"category_id"`
	Qty            uint32 `json:"qty"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
	At             string `json:"at"`
}

type PaymentEventRow struct {
	PaymentID   uint64 `json:"payment_id"`
	Kind        string `json:"kind"` // succeeded | failed
	CheckoutID  string `json:"checkout_id"`
	BuyerID     uint64 `json:"buyer_id"`
	Method      string `json:"method"`
	AmountMinor int64  `json:"amount_minor"`
	FailureCode string `json:"failure_code"`
	Terminal    uint8  `json:"terminal"`
	At          string `json:"at"`
}

type RefundRow struct {
	RefundID    uint64 `json:"refund_id"`
	PaymentID   uint64 `json:"payment_id"`
	CheckoutID  string `json:"checkout_id"`
	OrderID     uint64 `json:"order_id"`
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason"`
	At          string `json:"at"`
}

type ProductRow struct {
	ProductID  uint64 `json:"product_id"`
	StoreID    uint64 `json:"store_id"`
	Name       string `json:"name"`
	SKU        string `json:"sku"`
	CategoryID uint64 `json:"category_id"`
	Brand      string `json:"brand"`
	PriceMinor int64  `json:"price_minor"`
	Status     string `json:"status"`
	UpdatedAt  string `json:"updated_at"`
}

type InventoryRow struct {
	ProductID uint64 `json:"product_id"`
	StoreID   uint64 `json:"store_id"`
	Delta     int64  `json:"delta"`
	Available int64  `json:"available"`
	Reserved  int64  `json:"reserved"`
	Level     string `json:"level"`
	Reason    string `json:"reason"`
	RefType   string `json:"ref_type"`
	RefID     string `json:"ref_id"`
	At        string `json:"at"`
}
