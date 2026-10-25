// Package migrations nhúng các file SQL goose của product-service vào binary.
package migrations

import "embed"

// FS chứa các migration NNNNN_<tên>.sql.
//
//go:embed *.sql
var FS embed.FS
