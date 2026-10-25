package store

import "fmt"

// OrdersCTE returns the "orders AS (...)" common table expression (one row per order, ADR-22 revenue columns
// recognized / rec_at) so other tools (reconcile) use the single revenue definition. where is an optional
// "WHERE ..." filter on fact_order_status ("" for the whole platform).
func OrdersCTE(where string) string { return fmt.Sprintf(ordersCTE, where) }
