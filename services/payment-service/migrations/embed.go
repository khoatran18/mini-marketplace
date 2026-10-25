// Package migrations nhúng các file SQL goose của payment-service.
package migrations

import "embed"

// FS chứa các file NNNNN_<tên>.sql (-- +goose Up / Down).
//
//go:embed *.sql
var FS embed.FS
