// Package migrations nhúng các file SQL goose của auth-service.
package migrations

import "embed"

// FS chứa các file *.sql (NNNNN_<tên>.sql).
//
//go:embed *.sql
var FS embed.FS
