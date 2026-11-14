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
type ToolParam struct {
	Name        string   `json:"name"`
	In          string   `json:"in"`
	Type        string   `json:"type"` // string | integer | number
	Required    bool     `json:"required,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// Scopes maps role -> tools.
type Scopes map[string][]Tool

// DefaultScopes is the table in use: buyer, seller, admin and dev (docs/ai/08_AGENT_BUILD_ROADMAP.md §5). Every row is a GET: writes are never reachable with an agent token, they go
// through a signed proposal and the user's own token. There is deliberately no row for /auth/*, /users/*, addresses,
// /payments/:id or any /admin route.
var DefaultScopes = Scopes{
	"buyer":           buyerTools,
	"seller_admin":    sellerTools,
	"seller_employee": sellerTools,
	"admin":           adminTools,
	"dev":             devTools,
}

var buyerTools = []Tool{
	{Name: "search_products", Method: "GET", Path: "/search", Description: "Search active products by keyword, optionally by category.", Params: []ToolParam{
		{Name: "q", In: "query", Type: "string", Description: "keywords"},
		{Name: "category_id", In: "query", Type: "integer"},
		{Name: "price_min", In: "query", Type: "number"},
		{Name: "price_max", In: "query", Type: "number"},
		{Name: "brand", In: "query", Type: "string"},
		{Name: "in_stock", In: "query", Type: "string", Enum: []string{"true", "false"}},
		{Name: "sort", In: "query", Type: "string", Enum: []string{"relevance", "price_asc", "price_desc", "newest", "best_selling"}},
		{Name: "page", In: "query", Type: "integer"},
		{Name: "page_size", In: "query", Type: "integer", Description: "max 50"},
	}},
	{Name: "get_product", Method: "GET", Path: "/products/:id", Description: "Details of one product.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "get_product_stock", Method: "GET", Path: "/products/:id/stock", Description: "Available quantity of one product.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "list_categories", Method: "GET", Path: "/categories", Description: "Product categories."},
	{Name: "get_cart", Method: "GET", Path: "/cart", Description: "The user's own cart with totals."},
	{Name: "list_my_orders", Method: "GET", Path: "/orders", Description: "The user's own orders, newest first.", Params: []ToolParam{
		{Name: "status", In: "query", Type: "string"},
		{Name: "page", In: "query", Type: "integer"},
		{Name: "page_size", In: "query", Type: "integer"},
	}},
	{Name: "count_my_orders", Method: "GET", Path: "/orders/summary", Description: "Number of the user's own orders per status."},
	{Name: "get_my_order", Method: "GET", Path: "/orders/:id", Description: "One of the user's own orders (the service refuses other people's orders).", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "get_shop_rules", Method: "GET", Path: "/shop/rules", Description: "The order rules in force right now: minutes to pay an online order, days to request a return after delivery, days until a shipped order counts as delivered, shipping fee per store order and the subtotal from which shipping is free (whole VND; 0 = never). Use it for EVERY number about these rules."},
	{Name: "list_payment_methods", Method: "GET", Path: "/payments/methods", Description: "Payment methods the shop offers (information only)."},
}

var analyticsParams = []ToolParam{
	{Name: "from", In: "query", Type: "string", Description: "start, RFC3339 or YYYY-MM-DD"},
	{Name: "to", In: "query", Type: "string", Description: "end, RFC3339 or YYYY-MM-DD"},
	{Name: "granularity", In: "query", Type: "string", Enum: []string{"hour", "day", "week", "month"}},
	{Name: "limit", In: "query", Type: "integer"},
}

// sellerTools: the shop's own data only. The store comes from the token's user (auth-service lookup), never from a
// parameter, so none of these takes a store id. Product browsing tools are the public ones (no owner data).
var sellerTools = []Tool{
	{Name: "get_shop_rules", Method: "GET", Path: "/shop/rules", Description: "The order rules in force right now: minutes to pay an online order, days to request a return after delivery, days until a shipped order counts as delivered, shipping fee per store order and the subtotal from which shipping is free (whole VND; 0 = never). Use it for EVERY number about these rules."},
	{Name: "list_my_products", Method: "GET", Path: "/seller/products", Description: "Products of the seller's own store, every status. For 'most/least stocked', 'best selling', 'newest' or 'cheapest' questions use sort with a small page_size (5) instead of paging through everything; the result's total tells how many products exist.", Params: []ToolParam{
		{Name: "q", In: "query", Type: "string"},
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
		{Name: "threshold", In: "query", Type: "integer"},
	}},
	{Name: "list_store_orders", Method: "GET", Path: "/seller/orders", Description: "Orders of the seller's own store.", Params: []ToolParam{
		{Name: "status", In: "query", Type: "string"},
		{Name: "page", In: "query", Type: "integer"},
		{Name: "page_size", In: "query", Type: "integer"},
	}},
	{Name: "count_store_orders", Method: "GET", Path: "/seller/orders/summary", Description: "Number of the store's orders per status."},
	{Name: "get_store_order", Method: "GET", Path: "/seller/orders/:id", Description: "One order of the store.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "analytics_summary", Method: "GET", Path: "/seller/analytics/summary", Description: "Store KPIs for a period (revenue, orders...). Includes as_of and source.", Params: analyticsParams},
	{Name: "analytics_timeseries", Method: "GET", Path: "/seller/analytics/timeseries", Description: "Store revenue/orders over time.", Params: analyticsParams},
	{Name: "analytics_top_products", Method: "GET", Path: "/seller/analytics/top-products", Description: "Best-selling products of the store.", Params: analyticsParams},
	{Name: "analytics_funnel", Method: "GET", Path: "/seller/analytics/funnel", Description: "View -> cart -> order funnel of the store.", Params: analyticsParams},
	{Name: "analytics_low_stock", Method: "GET", Path: "/seller/analytics/low-stock", Description: "Low-stock report from analytics (may lag Postgres; prefer list_low_stock for exact stock).", Params: analyticsParams},
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
}

const opsQueryHelp = "Run one named metrics/log lookup (Prometheus, Loki). Latency: gateway_latency_p95 (per route), grpc_latency_p95 (per service) - " +
	"use these for 'which service is slowest'. Errors: gateway_5xx_ratio, grpc_error_rate, logs_errors(service). Load: gateway_request_rate, grpc_request_rate. " +
	"State: alerts_firing, service_up/service_ready(service), service_replicas(_up), circuit_state, infra_up. Data flow: outbox_pending, outbox_oldest_age, " +
	"outbox_dead, dlq_increase, kafka_consumer_lag. Resources: memory_ratio, memory_growth, db_pool_wait, disk_free_ratio. Trace one request: logs_by_request(request_id)."

// opsTools: read-only view of the platform's health, shared by admin and dev (system data only, no business figures).
var opsTools = []Tool{
	{Name: "ops_list_incidents", Method: "GET", Path: "/ops/incidents", Description: "Incidents raised from alerts, newest first.", Params: []ToolParam{
		{Name: "state", In: "query", Type: "string", Enum: []string{"open", "investigating", "resolved"}}, limitParam,
	}},
	{Name: "ops_get_incident", Method: "GET", Path: "/ops/incidents/:id", Description: "One incident with its runbook and suggested action.", Params: []ToolParam{
		{Name: "id", In: "path", Type: "integer", Required: true},
	}},
	{Name: "ops_list_audit", Method: "GET", Path: "/ops/audit", Description: "Append-only audit log of ops decisions. subject_type and subject_id go together.", Params: []ToolParam{
		{Name: "subject_type", In: "query", Type: "string"}, {Name: "subject_id", In: "query", Type: "string"}, limitParam,
	}},
	{Name: "ops_outbox_dead", Method: "GET", Path: "/ops/outbox/:service/dead", Description: "Outbox events that failed for good in one service (order, product or payment).", Params: []ToolParam{
		{Name: "service", In: "path", Type: "string", Required: true, Description: "service name, e.g. order-service"}, limitParam,
	}},
	{Name: "ops_list_proposals", Method: "GET", Path: "/ops/proposals", Description: "Ops proposals (actions waiting for a human review/approval, or already decided). Read only: nobody can approve or run one through the agent.", Params: []ToolParam{
		{Name: "state", In: "query", Type: "string", Enum: []string{"pending", "accepted", "rejected", "needs_review", "alternative", "expired"}},
		{Name: "incident_id", In: "query", Type: "integer"}, limitParam,
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
	{Name: "ops_system_health", Method: "GET", Path: "/ops/system/health", Description: "Live health of every service."},
	{Name: "data_health", Method: "GET", Path: "/admin/analytics/data-health", Description: "Analytics pipeline lag and reconcile state (not business figures)."},
}

// devTools: the system only. A dev must not see business data, same rule as the gateway routes.
var devTools = opsTools

var adminReportParams = append([]ToolParam{}, analyticsParams...)

// adminTools: platform-wide reads (every store) plus the ops view. No accounts, no password or user routes.
var adminTools = append([]Tool{
	{Name: "admin_list_orders", Method: "GET", Path: "/admin/orders", Description: "Orders of every store, newest first.", Params: append([]ToolParam{{Name: "status", In: "query", Type: "string"}}, pageParams...)},
	{Name: "admin_get_order", Method: "GET", Path: "/admin/orders/:id", Description: "One order of any store.", Params: []ToolParam{{Name: "id", In: "path", Type: "integer", Required: true}}},
	{Name: "admin_list_payments", Method: "GET", Path: "/admin/payments", Description: "Payments (simulated), newest first.", Params: append([]ToolParam{
		{Name: "status", In: "query", Type: "string"}, {Name: "method", In: "query", Type: "string"}}, pageParams...)},
	{Name: "admin_get_payment", Method: "GET", Path: "/admin/payments/:id", Description: "One payment with its attempts and refunds.", Params: []ToolParam{{Name: "id", In: "path", Type: "integer", Required: true}}},
	{Name: "admin_list_categories", Method: "GET", Path: "/admin/categories", Description: "All categories, including inactive ones."},
	{Name: "admin_analytics_summary", Method: "GET", Path: "/admin/analytics/summary", Description: "Platform KPIs for a period. Includes as_of and source.", Params: adminReportParams},
	{Name: "admin_analytics_timeseries", Method: "GET", Path: "/admin/analytics/timeseries", Description: "Platform revenue/orders over time.", Params: adminReportParams},
	{Name: "admin_analytics_top_products", Method: "GET", Path: "/admin/analytics/top-products", Description: "Best-selling products across the platform.", Params: adminReportParams},
	{Name: "admin_analytics_funnel", Method: "GET", Path: "/admin/analytics/funnel", Description: "View -> cart -> order funnel.", Params: adminReportParams},
	{Name: "admin_analytics_low_stock", Method: "GET", Path: "/admin/analytics/low-stock", Description: "Low-stock report across stores.", Params: adminReportParams},
	{Name: "admin_analytics_traffic", Method: "GET", Path: "/admin/analytics/traffic", Description: "Visits and sources.", Params: adminReportParams},
	{Name: "admin_analytics_payments", Method: "GET", Path: "/admin/analytics/payments", Description: "Payment success/failure by method.", Params: adminReportParams},
	{Name: "admin_analytics_search_terms", Method: "GET", Path: "/admin/analytics/search-terms", Description: "Most searched terms and those with no result.", Params: adminReportParams},
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

// readOnlyRoles get read tools only: no propose_ tools are served and /ai/proposals refuses them, although DefaultActions keeps
// the admin rows so the proposable list and its confirm gate stay covered by tests. Remove a role here to let its agent propose.
var readOnlyRoles = map[string]bool{"admin": true, "dev": true}

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
