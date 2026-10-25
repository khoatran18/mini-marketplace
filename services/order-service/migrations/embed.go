// Package migrations nhúng các file SQL goose của order-service.
package migrations

import "embed"

// FS chứa mọi file NNNNN_<tên>.sql.
//
//go:embed *.sql
var FS embed.FS
