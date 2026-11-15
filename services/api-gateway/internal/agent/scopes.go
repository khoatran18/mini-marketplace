package agent

import (
	"regexp"
	"sort"
	"strings"
)

// Tool is one thing the agent may do for a role: a named, read-only call to a normal gateway route. This table is the single
// source of truth (ADR-42): the gateway lets an agent token through a route only if the route is listed here for the token's
// role, GET /ai/tools serves the same table to ai-agent, and the router test checks every row points at a real route.
type Tool struct {
	Name        string      `json:"name"`
	Method      string      `json:"method"`
	Path        string      `json:"path"` // gin route pattern, e.g. /orders/:id
	Description string      `json:"description"`
	Params      []ToolParam `json:"params,omitempty"`
}

// Action is a write the agent may only PROPOSE for a role (never execute): the user confirms it in the UI, which replays the
// real route with the user's own token (ADR-42). Params have In "path" or "body". Summary is the sentence the user reads on the
// confirmation card; {name} placeholders are filled by ai-agent from the validated params, so the text always matches what will run.
type Action struct {
	Name        string      `json:"name"`
	Method      string      `json:"method"`
	Path        string      `json:"path"` // gin route pattern, e.g. /orders/:id/cancel
	Description string      `json:"description"`
	Summary     string      `json:"summary"`
	Params      []ToolParam `json:"params,omitempty"`
}

// ToolParam describes one input of a tool or action; In is "path", "query" or (actions only) "body".
// Nest (actions only, body params) puts the value under that key of the body, e.g. Nest "params" gives {"params":{"service":"x"}}.
type ToolParam struct {
	Name        string   `json:"name"`
	In          string   `json:"in"`
	Type        string   `json:"type"` // string | integer | number | string_list | integer_list (lists: actions only)
	Nest        string   `json:"nest,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// Scopes maps role -> tools.
type Scopes map[string][]Tool

// DefaultScopes is the table in use: buyer, seller, admin and operator (docs/ai/08_AGENT_BUILD_ROADMAP.md §5). Every row is a GET: writes are never reachable with an agent token, they go
// through a signed proposal and the user's own token. There is deliberately no row for /auth/*, /users/*, addresses,
// /payments/:id or any /admin route.
var DefaultScopes = Scopes{
	"buyer":           buyerTools,
	"seller_admin":    sellerTools,
	"seller_employee": sellerTools,
	"admin":           adminTools,
	"operator":        operatorTools,
}

// Filters of the order lists (one gateway handler serves buyer, seller and admin). Every filter is optional: leaving it out means "no filter".
// A tool param exists only if the handler reads it (TestEveryQueryParamOfAToolIsReadByAHandler); values outside the enums are rejected by the
// order-service with 400, never silently ignored.
var (
	orderStatuses = []string{"PENDING", "AWAITING_PAYMENT", "CONFIRMED", "PAID", "SHIPPED", "DELIVERED", "REFUND_REQUESTED", "REFUNDED", "CANCELED", "EXPIRED", "FAILED"}
	// Meaning of each value, from the order state machine (docs/design/02_DATA_MODEL.md): the model maps everyday words to these, so say what each one is.
	orderStatusHelp = "PENDING just created; AWAITING_PAYMENT online order not paid yet; CONFIRMED COD order accepted; PAID online order paid; " +
		"CONFIRMED and PAID both mean accepted and not shipped yet; SHIPPED handed to the carrier; DELIVERED received; REFUND_REQUESTED the buyer asked to return, waiting for the shop; " +
		"REFUNDED return approved and refunded; CANCELED cancelled; EXPIRED unpaid until the payment deadline; FAILED could not be created"
	orderFilters = []ToolParam{
		{Name: "status", In: "query", Type: "string", Enum: orderStatuses, Description: orderStatusHelp},
		{Name: "payment_method", In: "query", Type: "string", Enum: []string{"COD", "MOCK_CARD", "MOCK_WALLET", "MOCK_BANK_TRANSFER"}, Description: "COD = cash on delivery; MOCK_CARD = card; MOCK_WALLET = e-wallet; MOCK_BANK_TRANSFER = bank transfer (all payments are simulated)"},
		{Name: "payment_status", In: "query", Type: "string", Enum: []string{"UNPAID", "PAID", "REFUNDED"}, Description: "UNPAID = money not collected yet; PAID = collected; REFUNDED = money returned"},
		{Name: "created_from", In: "query", Type: "string", Description: "orders created on or after this day, YYYY-MM-DD (Asia/Ho_Chi_Minh) or RFC3339"},
		{Name: "created_to", In: "query", Type: "string", Description: "orders created up to and including this day, YYYY-MM-DD or RFC3339"},
		{Name: "min_total", In: "query", Type: "number", Description: "order total at least this, in VND"},
		{Name: "max_total", In: "query", Type: "number", Description: "order total at most this, in VND"},
		{Name: "sort", In: "query", Type: "string", Enum: []string{"newest", "oldest", "total_desc", "total_asc"}, Description: "default newest; total_desc = biggest orders first"},
		{Name: "page", In: "query", Type: "integer"},
		{Name: "page_size", In: "query", Type: "integer", Description: "default 20, max 100; use 5 for top-N questions"},
	}
	// rankingParam: how analytics top-products ranks (analytics-service store.productSorts).
	rankingParam = ToolParam{Name: "sort", In: "query", Type: "string", Enum: []string{"revenue", "units", "views", "conversion", "viewed_unsold"}, Description: "ranking; default revenue"}
)

var buyerTools = []Tool{
	{Name: "search_products", Method: "GET", Path: "/search", Description: "Search active products by keyword. Every filter (category, price range, brand, in stock, sort) is optional and can be combined in ONE call, so a price or brand condition is a parameter, not a second search.", Params: []ToolParam{
		{Name: "q", In: "query", Type: "string", Description: "keywords"},
		{Name: "category_id", In: "query", Type: "integer", Description: "only this category (ids from list_categories)"},
		{Name: "price_min", In: "query", Type: "number", Description: "price at least this, in VND"},
		{Name: "price_max", In: "query", Type: "number", Description: "price at most this, in VND"},
		{Name: "brand", In: "query", Type: "string", Description: "exact brand name"},
		{Name: "in_stock", In: "query", Type: "string", Enum: []string{"true", "false"}, Description: "true = only products that can be bought now"},
		{Name: "sort", In: "query", Type: "string", Enum: []string{"relevance", "price_asc", "price_desc", "newest", "best_selling"}, Description: "default relevance; price_asc = cheapest first, price_desc = most expensive first"},
		{Name: "page", In: "query", Type: "integer"},
		{Name: "page_size", In: "query", Type: "integer", Description: "max 50"},
	}},
	{Name: "get_product", Method: "GET", Path: "/products/:id", Description: "Details of one product.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "get_product_stock", Method: "GET", Path: "/products/:id/stock", Description: "Units available to buy right now for one product (large quantities are shown as a capped figure such as 20+).", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "list_categories", Method: "GET", Path: "/categories", Description: "Product categories."},
	{Name: "get_cart", Method: "GET", Path: "/cart", Description: "The user's own cart with totals."},
	{Name: "list_my_orders", Method: "GET", Path: "/orders", Description: "The user's own orders, newest first by default. Filter instead of paging: status, payment method/status, date range, total range, sort. The result's total says how many orders match.", Params: orderFilters},
	{Name: "count_my_orders", Method: "GET", Path: "/orders/summary", Description: "Number of the user's own orders: how many orders are in each status (by_status); add them up for the overall count. One call answers 'how many orders' questions."},
	{Name: "get_my_order", Method: "GET", Path: "/orders/:id", Description: "One of the user's own orders (the service refuses other people's orders).", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "get_shop_rules", Method: "GET", Path: "/shop/rules", Description: "The order rules in force right now: minutes to pay an online order, days to request a return after delivery, days until a shipped order counts as delivered, shipping fee per store order and the subtotal from which shipping is free (whole VND; 0 = never). Use it for EVERY number about these rules."},
	{Name: "list_payment_methods", Method: "GET", Path: "/payments/methods", Description: "Payment methods the shop offers (information only)."},
}

// Analytics reports. A param is offered only to the reports that read it (analytics-service/internal/service/service.go): `granularity` only changes
// the series of timeseries and traffic, `limit` only the row count of top_products, low_stock and search_terms. Offering them everywhere made the model
// pass granularity=month to a summary and believe it had asked for "this month" (the period comes from from/to; without `from` it is the last 30 days).
var (
	analyticsFrom = ToolParam{Name: "from", In: "query", Type: "string", Required: true, Description: "REQUIRED. First day of the period, YYYY-MM-DD (Asia/Ho_Chi_Minh) or RFC3339. Work it out from today's date: today = today, this week = Monday, this month = the 1st, last 30 days = today minus 30. Set `to` as well unless the period ends now"}
	analyticsTo   = ToolParam{Name: "to", In: "query", Type: "string", Description: "last day of the period, inclusive, YYYY-MM-DD or RFC3339; default now"}
	analyticsGran = ToolParam{Name: "granularity", In: "query", Type: "string", Enum: []string{"hour", "day", "week", "month"}, Description: "size of one point in the series (default day). It does not choose the period: use from/to for that"}
	analyticsRows = ToolParam{Name: "limit", In: "query", Type: "integer", Description: "max rows to return"}

	periodParams = []ToolParam{analyticsFrom, analyticsTo}
	seriesParams = []ToolParam{analyticsFrom, analyticsTo, analyticsGran}
	rankedParams = []ToolParam{analyticsFrom, analyticsTo, analyticsRows}
)

// sellerTools: the shop's own data only. The store comes from the token's user (auth-service lookup), never from a
// parameter, so none of these takes a store id. Product browsing tools are the public ones (no owner data).
var sellerTools = []Tool{
	{Name: "get_shop_rules", Method: "GET", Path: "/shop/rules", Description: "The order rules in force right now: minutes to pay an online order, days to request a return after delivery, days until a shipped order counts as delivered, shipping fee per store order and the subtotal from which shipping is free (whole VND; 0 = never). Use it for EVERY number about these rules."},
	{Name: "list_my_products", Method: "GET", Path: "/seller/products", Description: "Products of the seller's own store, every status. For 'most/least stocked', 'best selling', 'newest' or 'cheapest' questions use sort with a small page_size (5) instead of paging through everything; the result's total tells how many products exist.", Params: []ToolParam{
		{Name: "q", In: "query", Type: "string", Description: "keywords in the product name"},
		{Name: "category_id", In: "query", Type: "integer", Description: "only this category"},
		{Name: "brand", In: "query", Type: "string", Description: "exact brand name"},
		{Name: "price_min", In: "query", Type: "number", Description: "price at least this, in VND"},
		{Name: "price_max", In: "query", Type: "number", Description: "price at most this, in VND"},
		{Name: "sort", In: "query", Type: "string", Enum: []string{"stock_desc", "stock_asc", "best_selling", "newest", "price_asc", "price_desc"}, Description: "stock_desc = most available stock first"},
		{Name: "in_stock", In: "query", Type: "string", Enum: []string{"true"}, Description: "only products with stock above 0"},
		{Name: "page", In: "query", Type: "integer"},
		{Name: "page_size", In: "query", Type: "integer", Description: "default 20, max 50; use 5 for top-N questions"},
	}},
	{Name: "get_product", Method: "GET", Path: "/products/:id", Description: "Details of one product.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "get_inventory_ledger", Method: "GET", Path: "/seller/products/:id/inventory/ledger", Description: "Stock movements of one of the store's products, newest first.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
		{Name: "page", In: "query", Type: "integer"},
		{Name: "page_size", In: "query", Type: "integer"},
	}},
	{Name: "list_low_stock", Method: "GET", Path: "/seller/inventory/low-stock", Description: "The store's products at or below their low-stock threshold.", Params: []ToolParam{
		{Name: "threshold", In: "query", Type: "integer", Description: "list products with at most this many units available; leave out to use each product's own low-stock threshold"},
	}},
	{Name: "list_store_orders", Method: "GET", Path: "/seller/orders", Description: "Orders of the seller's own store, newest first by default. Filter instead of paging: status, payment method/status, date range, total range, sort. The result's total says how many orders match.", Params: orderFilters},
	{Name: "count_store_orders", Method: "GET", Path: "/seller/orders/summary", Description: "Number of the store's orders: how many orders are in each status (by_status); add them up for the overall count. One call answers 'how many orders' questions."},
	{Name: "get_store_order", Method: "GET", Path: "/seller/orders/:id", Description: "One order of the store.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "analytics_summary", Method: "GET", Path: "/seller/analytics/summary", Description: "KPIs of the store for ONE period: revenue (goods, before refunds), refunds, net_revenue, gmv, orders_placed, orders_recognized, aov, canceled, expired, refunded, cancel_rate, plus the same figures for the previous period of equal length. Revenue counts orders PAID (online) or DELIVERED (COD). Default period is the last 30 days. Includes as_of and source.", Params: periodParams},
	{Name: "analytics_timeseries", Method: "GET", Path: "/seller/analytics/timeseries", Description: "Revenue, orders and refunds of the store per time bucket (granularity) over the period, empty buckets as 0. Use it for by-day / by-week / by-month questions; use analytics_summary for a single total.", Params: seriesParams},
	{Name: "analytics_top_products", Method: "GET", Path: "/seller/analytics/top-products", Description: "Products of the store ranked by revenue (default), units, views, conversion, or viewed_unsold (many views, no sale).", Params: append(append([]ToolParam{}, rankedParams...), rankingParam)},
	{Name: "analytics_funnel", Method: "GET", Path: "/seller/analytics/funnel", Description: "Distinct sessions per stage view -> cart -> checkout, and the recognised orders, for the period. The checkout stage is null for one store (not attributable).", Params: periodParams},
	{Name: "analytics_low_stock", Method: "GET", Path: "/seller/analytics/low-stock", Description: "Products that are out of stock, low, or would sell out within 7 days at the last 14 days' sales rate (analytics, may lag Postgres; prefer list_low_stock for exact stock). Not tied to a period.", Params: []ToolParam{analyticsRows}},
}

var limitParam = ToolParam{Name: "limit", In: "query", Type: "integer", Description: "max rows"}
var pageParams = []ToolParam{{Name: "page", In: "query", Type: "integer"}, {Name: "page_size", In: "query", Type: "integer"}}

var opsServices = []string{"api-gateway", "auth-service", "user-service", "product-service", "order-service", "payment-service", "analytics-service", "ops-service"}

// opsQueryNames mirrors the catalog in ops-service (internal/queries): fixed Prometheus/Loki lookups, the agent never writes PromQL/LogQL.
var opsQueryNames = []string{
	"alerts_firing", "service_up", "service_ready", "service_replicas", "service_replicas_up", "gateway_request_rate", "grpc_request_rate",
	"grpc_error_rate", "circuit_state", "outbox_pending", "outbox_oldest_age", "outbox_dead", "gateway_5xx_ratio", "dlq_increase",
	"memory_ratio", "kafka_consumer_lag", "gateway_latency_p95", "grpc_latency_p95", "db_pool_wait", "disk_free_ratio", "memory_growth",
	"infra_up", "logs_errors", "logs_by_request",
	"frontend_request_rate", "frontend_5xx_ratio", "frontend_latency_p95",
	"ai_runs_rate", "ai_tokens_rate", "ai_tool_errors", "ai_llm_latency_p95", "ai_rejected",
}

const opsQueryHelp = "Run one named metrics/log lookup (Prometheus, Loki). Latency: gateway_latency_p95 (per route), grpc_latency_p95 (per service) - " +
	"use these for 'which service is slowest'. Errors: gateway_5xx_ratio, grpc_error_rate, logs_errors(service). Load: gateway_request_rate, grpc_request_rate. " +
	"State: alerts_firing, service_up/service_ready(service), service_replicas(_up), circuit_state, infra_up. Data flow: outbox_pending, outbox_oldest_age, " +
	"outbox_dead, dlq_increase, kafka_consumer_lag. Resources: memory_ratio, memory_growth, db_pool_wait, disk_free_ratio. Trace one request: logs_by_request(request_id). " +
	"Frontend (measured at Traefik): frontend_request_rate, frontend_5xx_ratio, frontend_latency_p95. The AI assistant itself: ai_runs_rate, ai_tokens_rate, ai_tool_errors, ai_llm_latency_p95, ai_rejected."

// opsTools: read-only view of the platform's health, shared by admin and operator (system data only, no business figures).
var opsTools = []Tool{
	{Name: "ops_list_incidents", Method: "GET", Path: "/ops/incidents", Description: "Incidents opened automatically from firing alerts, newest first. Filter by state; the list already says which alert and how severe.", Params: []ToolParam{
		{Name: "state", In: "query", Type: "string", Enum: []string{"open", "investigating", "resolved"}, Description: "open = nobody has taken it yet; investigating = someone is working on it; resolved = finished (alert gone or closed)"}, limitParam,
	}},
	{Name: "ops_get_incident", Method: "GET", Path: "/ops/incidents/:id", Description: "One incident with its runbook and suggested action.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "ops_list_audit", Method: "GET", Path: "/ops/audit", Description: "Append-only audit log of ops decisions. subject_type and subject_id go together.", Params: []ToolParam{
		{Name: "subject_type", In: "query", Type: "string", Description: "what the decision was about, e.g. incident or proposal"}, {Name: "subject_id", In: "query", Type: "string", Description: "id of that incident or proposal"}, limitParam,
	}},
	{Name: "ops_outbox_dead", Method: "GET", Path: "/ops/outbox/:service/dead", Description: "Outbox events that failed for good in one service (order, product or payment).", Params: []ToolParam{
		{Name: "service", In: "path", Type: "string", Required: true, Description: "service name, e.g. order-service"}, limitParam,
	}},
	{Name: "ops_list_actions", Method: "GET", Path: "/ops/actions", Description: "The closed catalog of operational actions (id, title, risk level observe/low/high, mode, parameters). Read it before proposing one: an action that is not in it cannot be proposed.", Params: nil},
	{Name: "ops_list_proposals", Method: "GET", Path: "/ops/proposals", Description: "Ops proposals (actions waiting for a human review/approval, or already decided). Read only: nobody can approve or run one through the agent.", Params: []ToolParam{
		{Name: "state", In: "query", Type: "string", Enum: []string{"pending", "accepted", "rejected", "needs_review", "alternative", "expired"}, Description: "pending = waiting for a human; accepted / rejected = decided; needs_review = the human asked for another look; alternative = the human chose a different action; expired = nobody decided in time"},
		{Name: "incident_id", In: "query", Type: "integer", Description: "only proposals made for this incident"}, limitParam,
	}},
	{Name: "ops_get_proposal", Method: "GET", Path: "/ops/proposals/:id", Description: "One ops proposal with its reasoning, review notes and decision.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "ops_metrics_query", Method: "GET", Path: "/ops/queries/:name", Description: opsQueryHelp, Params: []ToolParam{
		{Name: "name", In: "path", Type: "string", Required: true, Enum: opsQueryNames, Description: "which named query to run"},
		{Name: "service", In: "query", Type: "string", Enum: opsServices, Description: "needed by service_up, service_ready, logs_errors"},
		{Name: "window", In: "query", Type: "string", Enum: []string{"5m", "15m", "1h"}, Description: "look-back window; each query has its own default"},
		{Name: "request_id", In: "query", Type: "string", Description: "only for logs_by_request: the X-Request-ID to follow across services"},
	}},
	{Name: "ops_list_playbooks", Method: "GET", Path: "/ops/playbooks", Description: "Incident playbooks (ground truth): which alerts each covers. Pass alert=<AlertName> to find the playbook of a firing alert. Read the playbook BEFORE proposing a cause.", Params: []ToolParam{
		{Name: "alert", In: "query", Type: "string", Description: "alert name, e.g. OutboxDeadRows (from alerts_firing or an incident)"},
	}},
	{Name: "ops_get_playbook", Method: "GET", Path: "/ops/playbooks/:id", Description: "One playbook: diagnose steps (read-only queries to run with ops_metrics_query), causes with the evidence that points to each, and the safe actions with their risk level. The actions are for the human to carry out; you cannot run them.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "string", Required: true, Description: "playbook id from ops_list_playbooks, e.g. outbox-stuck"},
	}},
	{Name: "ops_system_health", Method: "GET", Path: "/ops/system/health", Description: "Live readiness of every service right now (each one answers ok or not), one call for the whole system."},
	{Name: "data_health", Method: "GET", Path: "/admin/analytics/data-health", Description: "Analytics pipeline lag and reconcile state (not business figures)."},
}

// operatorTools: the system only. An operator must not see business data, same rule as the gateway routes.
var operatorTools = opsTools

// adminReportParams was a copy of the generic analytics params; each admin report now takes only what it reads (see above).

// adminTools: platform-wide reads (every store) plus the ops view. No accounts, no password or user routes.
var adminTools = append([]Tool{
	{Name: "admin_list_orders", Method: "GET", Path: "/admin/orders", Description: "Orders of every store, newest first by default. Filter instead of paging: status, payment method/status, date range, total range, one store, sort. The result's total says how many orders match.", Params: append(append([]ToolParam{}, orderFilters...),
		ToolParam{Name: "store_id", In: "query", Type: "integer", Description: "only this store's orders"})},
	{Name: "admin_get_order", Method: "GET", Path: "/admin/orders/:id", Description: "One order of any store.", Params: []ToolParam{{Name: "id", In: "path", Type: "integer", Required: true}}},
	{Name: "admin_list_payments", Method: "GET", Path: "/admin/payments", Description: "Payments (simulated), newest first by default. Filter instead of paging: status, method, date range, amount range, sort. The result's total says how many payments match.", Params: []ToolParam{
		{Name: "status", In: "query", Type: "string", Enum: []string{"REQUIRES_ACTION", "PROCESSING", "SUCCEEDED", "FAILED", "CANCELED", "EXPIRED", "PARTIALLY_REFUNDED", "REFUNDED"}, Description: "REQUIRES_ACTION waiting for the payer; PROCESSING sent, waiting for the result; SUCCEEDED money collected; FAILED gave up after repeated errors; CANCELED by the buyer; EXPIRED past the deadline; PARTIALLY_REFUNDED / REFUNDED part or all of the money returned"},
		{Name: "method", In: "query", Type: "string", Enum: []string{"MOCK_CARD", "MOCK_WALLET", "MOCK_BANK_TRANSFER"}, Description: "MOCK_CARD = card; MOCK_WALLET = e-wallet; MOCK_BANK_TRANSFER = bank transfer (simulated)."},
		{Name: "created_from", In: "query", Type: "string", Description: "created on or after this day, YYYY-MM-DD (Asia/Ho_Chi_Minh) or RFC3339"},
		{Name: "created_to", In: "query", Type: "string", Description: "created up to and including this day, YYYY-MM-DD or RFC3339"},
		{Name: "min_amount", In: "query", Type: "number", Description: "amount at least this, in VND"},
		{Name: "max_amount", In: "query", Type: "number", Description: "amount at most this, in VND"},
		{Name: "sort", In: "query", Type: "string", Enum: []string{"newest", "oldest", "amount_desc", "amount_asc"}, Description: "default newest; amount_desc = largest payments first"},
		{Name: "page", In: "query", Type: "integer"},
		{Name: "page_size", In: "query", Type: "integer", Description: "default 20, max 100; use 5 for top-N questions"},
	}},
	{Name: "admin_get_payment", Method: "GET", Path: "/admin/payments/:id", Description: "One payment with its attempts and refunds.", Params: []ToolParam{{Name: "id", In: "path", Type: "integer", Required: true}}},
	{Name: "admin_list_categories", Method: "GET", Path: "/admin/categories", Description: "All categories, including inactive ones."},
	{Name: "admin_analytics_summary", Method: "GET", Path: "/admin/analytics/summary", Description: "Platform KPIs for ONE period: revenue, refunds, net_revenue, gmv, orders_placed, orders_recognized, aov, canceled, expired, refunded, cancel_rate, plus the previous period of equal length. Default period is the last 30 days. Includes as_of and source.", Params: periodParams},
	{Name: "admin_analytics_timeseries", Method: "GET", Path: "/admin/analytics/timeseries", Description: "Platform revenue, orders and refunds per time bucket (granularity) over the period. Use it for by-day / by-week / by-month questions; use admin_analytics_summary for one total.", Params: seriesParams},
	{Name: "admin_analytics_top_products", Method: "GET", Path: "/admin/analytics/top-products", Description: "Products across the platform ranked by revenue (default), units, views, conversion, or viewed_unsold (many views, no sale).", Params: append(append([]ToolParam{}, rankedParams...), rankingParam)},
	{Name: "admin_analytics_funnel", Method: "GET", Path: "/admin/analytics/funnel", Description: "Distinct sessions per stage view -> cart -> checkout and the recognised orders, for the period, across the platform.", Params: periodParams},
	{Name: "admin_analytics_low_stock", Method: "GET", Path: "/admin/analytics/low-stock", Description: "Low-stock report across stores (analytics, may lag): products out, low, or selling out within 7 days. Not tied to a period.", Params: []ToolParam{analyticsRows}},
	{Name: "admin_analytics_traffic", Method: "GET", Path: "/admin/analytics/traffic", Description: "Page views, sessions, unique visitors, new vs returning visitors, a series per granularity and top pages/sources, for the period.", Params: seriesParams},
	{Name: "admin_analytics_payments", Method: "GET", Path: "/admin/analytics/payments", Description: "Payment attempt outcomes by method, failure codes, refunds and orders awaiting payment, for the period. For individual payments use admin_list_payments.", Params: periodParams},
	{Name: "admin_analytics_search_terms", Method: "GET", Path: "/admin/analytics/search-terms", Description: "Most searched terms and the terms that returned nothing, for the period.", Params: rankedParams},
}, opsTools...)

// Tools returns the tools of a role (nil for an unknown role: no tools, no access).
func (s Scopes) Tools(role string) []Tool { return s[role] }

// Allows reports whether role may call method on the route pattern (c.FullPath()). Only exact matches count.
func (s Scopes) Allows(role, method, pattern string) bool {
	for _, t := range s[role] {
		if t.Method == method && t.Path == pattern {
			return true
		}
	}
	return false
}

// Routes lists every distinct (method, pattern) in the table, for tests that must check the routes exist.
func (s Scopes) Routes() [][2]string {
	seen := map[[2]string]bool{}
	var out [][2]string
	for _, tools := range s {
		for _, t := range tools {
			k := [2]string{t.Method, t.Path}
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][1]+out[i][0] < out[j][1]+out[j][0] })
	return out
}

var reasonParam = ToolParam{Name: "reason", In: "body", Type: "string", Description: "reason given by the user, in their own words (max 300 characters)"}
var requiredReason = ToolParam{Name: "reason", In: "body", Type: "string", Required: true, Description: "reason given by the user, in their own words (max 300 characters); ask the user if missing"}
var orderID = ToolParam{Name: "id", In: "path", Type: "integer", Required: true, Description: "order id"}

// DefaultActions: what each role's agent may propose. Every row must also be on the proposable list (proposal.go), which is the
// gateway's final check; roles are enforced again by the route when the user confirms.
var DefaultActions = map[string][]Action{
	"buyer": {
		{Name: "cancel_order", Method: "POST", Path: "/orders/:id/cancel", Summary: "Huỷ đơn hàng #{id}", Description: "Cancel one of the user's own orders (only while it is not shipped yet).", Params: []ToolParam{orderID, reasonParam}},
		{Name: "request_return", Method: "POST", Path: "/orders/:id/return", Summary: "Yêu cầu trả hàng cho đơn #{id}", Description: "Ask to return a delivered order inside the return window.", Params: []ToolParam{orderID, requiredReason}},
	},
	"seller_admin":    sellerActions,
	"seller_employee": sellerActions,
	// operator: one action, "create an ops proposal". Confirming it only records a proposal at ops-service; a different human must still
	// review and approve it, and only ops-service executes it (ADR-45).
	"operator": {opsProposalAction},
	// admin has no persona yet; the rows exist so the proposable list and its confirm gate stay covered by tests.
	"admin": {
		{Name: "cancel_order", Method: "POST", Path: "/admin/orders/:id/cancel", Summary: "Huỷ đơn #{id} (quản trị)", Description: "Cancel any order.", Params: []ToolParam{orderID, reasonParam}},
		{Name: "refund_payment", Method: "POST", Path: "/admin/payments/:id/refund", Summary: "Hoàn {amount_minor} (đơn vị nhỏ nhất) cho thanh toán #{id}", Description: "Refund part or all of a payment; amount_minor is in 1/100 VND.", Params: []ToolParam{
			{Name: "id", In: "path", Type: "integer", Required: true, Description: "payment id"},
			{Name: "amount_minor", In: "body", Type: "integer", Required: true, Description: "positive amount in 1/100 VND"},
			reasonParam,
		}},
	},
}

// opsActionIDs / opsActionServices mirror ops-service's closed catalog (services/ops-service/internal/actions/catalog.go); ops-service validates
// every proposal against its own catalog, so a drift here makes the proposal fail with 400, never run something else.
var (
	opsActionIDs      = []string{"requeue_outbox", "restart_service", "scale_service", "run_backfill", "escalate_to_human"}
	opsActionServices = []string{"api-gateway", "auth-service", "user-service", "product-service", "order-service", "payment-service", "analytics-service"}
)

var opsProposalAction = Action{
	Name: "ops_action", Method: "POST", Path: "/ops/proposals", Summary: "Đề xuất vận hành: {action_id}",
	Description: "Propose ONE operational action from the catalog (ops_list_actions). This only records a proposal for a human to review and approve; it runs nothing. Use only after you found the cause with evidence, and give that evidence.",
	Params: []ToolParam{
		{Name: "action_id", In: "body", Type: "string", Required: true, Enum: opsActionIDs, Description: "requeue_outbox: put DEAD outbox rows back (service + exactly one of ids / limit); restart_service: docker service update --force (service); scale_service: set replicas 1-5 (service + replicas); run_backfill: rebuild ClickHouse for up to 31 days (from + to); escalate_to_human: no automatic step, use when no playbook fits"},
		{Name: "incident_id", In: "body", Type: "integer", Description: "the incident this proposal is for, when there is one"},
		{Name: "rationale", In: "body", Type: "string", Required: true, Description: "why this action, in 1-3 sentences, based on the evidence"},
		{Name: "evidence", In: "body", Type: "string_list", Description: "up to 5 short facts you saw (query name, value, time) that justify the action"},
		{Name: "service", In: "body", Nest: "params", Type: "string", Enum: opsActionServices, Description: "for requeue_outbox (order-service, product-service or payment-service only), restart_service, scale_service"},
		{Name: "replicas", In: "body", Nest: "params", Type: "integer", Description: "for scale_service: 1 to 5"},
		{Name: "limit", In: "body", Nest: "params", Type: "integer", Description: "for requeue_outbox: how many dead rows (1-100); use this or ids, not both"},
		{Name: "ids", In: "body", Nest: "params", Type: "integer_list", Description: "for requeue_outbox: exact outbox row ids (1-100 of them) from ops_outbox_dead; use this or limit, not both"},
		{Name: "from", In: "body", Nest: "params", Type: "string", Description: "for run_backfill: first day YYYY-MM-DD"},
		{Name: "to", In: "body", Nest: "params", Type: "string", Description: "for run_backfill: last day YYYY-MM-DD (at most 31 days after from)"},
	},
}

var sellerActions = []Action{
	{Name: "cancel_order", Method: "POST", Path: "/seller/orders/:id/cancel", Summary: "Huỷ đơn #{id} của shop", Description: "Cancel one of the store's orders.", Params: []ToolParam{orderID, reasonParam}},
	{Name: "approve_return", Method: "POST", Path: "/seller/orders/:id/return/approve", Summary: "Duyệt trả hàng cho đơn #{id} (hoàn tiền cho người mua)", Description: "Approve a buyer's return request for one of the store's orders.", Params: []ToolParam{orderID}},
	{Name: "reject_return", Method: "POST", Path: "/seller/orders/:id/return/reject", Summary: "Từ chối trả hàng cho đơn #{id}", Description: "Reject a buyer's return request.", Params: []ToolParam{orderID, requiredReason}},
	{Name: "adjust_inventory", Method: "POST", Path: "/seller/products/:id/inventory/adjust", Summary: "Điều chỉnh tồn kho sản phẩm #{id}: {delta}", Description: "Manual stock correction of one of the store's products; delta is a signed whole number (+5 adds, -2 removes).", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true, Description: "product id"},
		{Name: "delta", In: "body", Type: "integer", Required: true, Description: "signed change in units, not zero"},
		requiredReason,
	}},
}

// readOnlyRoles get read tools only (the operator is not one: it may propose ops actions, ADR-45): no propose_ tools are served and /ai/proposals refuses them, although DefaultActions keeps
// the admin rows so the proposable list and its confirm gate stay covered by tests. Remove a role here to let its agent propose.
var readOnlyRoles = map[string]bool{"admin": true}

// ActionsFor returns the actions of a role's agent (nil for an unknown or read-only role).
func ActionsFor(role string) []Action {
	if readOnlyRoles[role] {
		return nil
	}
	return DefaultActions[role]
}

// CanPropose is the check /ai/proposals makes: the role's agent may propose, and this action is one of its own.
func CanPropose(role, method, path string) bool {
	return !readOnlyRoles[role] && AllowsAction(role, method, path)
}

func actionRegexp(pattern string) *regexp.Regexp {
	return regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `:id`, `\d+`) + "$")
}

// AllowsAction reports whether role's agent may propose method on this concrete path (e.g. /orders/12/cancel).
func AllowsAction(role, method, path string) bool {
	for _, a := range DefaultActions[role] {
		if a.Method == method && actionRegexp(a.Path).MatchString(path) {
			return true
		}
	}
	return false
}
