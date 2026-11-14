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

// DefaultScopes is the table in use: buyer and seller so far; admin/dev rows are added with their steps
// (docs/ai/08_AGENT_BUILD_ROADMAP.md §5). Every row is a GET: writes are never reachable with an agent token, they go
// through a signed proposal and the user's own token. There is deliberately no row for /auth/*, /users/*, addresses,
// /payments/:id or any /admin route.
var DefaultScopes = Scopes{
	"buyer":           buyerTools,
	"seller_admin":    sellerTools,
	"seller_employee": sellerTools,
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
	{Name: "list_my_products", Method: "GET", Path: "/seller/products", Description: "Products of the seller's own store, every status.", Params: []ToolParam{
		{Name: "q", In: "query", Type: "string"},
		{Name: "page", In: "query", Type: "integer"},
		{Name: "page_size", In: "query", Type: "integer"},
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

// ActionsFor returns the actions of a role (nil for an unknown role).
func ActionsFor(role string) []Action { return DefaultActions[role] }

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
