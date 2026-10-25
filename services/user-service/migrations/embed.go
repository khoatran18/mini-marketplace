// Package migrations nhúng các file SQL migration (goose) vào binary.
package migrations

import "embed"

// FS chứa mọi file NNNNN_<tên>.sql của user-service.
//
//go:embed *.sql
var FS embed.FS
