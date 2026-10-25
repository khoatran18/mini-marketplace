// Package migrate chạy migration goose (nhúng trong binary) cho user-service.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"user-service/migrations"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// VersionTable là bảng version riêng của user-service.
const VersionTable = "goose_db_version_user"

// Run áp dụng mọi migration chưa chạy. Dùng session lock Postgres để chỉ một replica chạy tại một thời điểm.
// Baseline dùng IF NOT EXISTS nên cũng an toàn trên DB cũ do AutoMigrate tạo (chưa có bảng version).
func Run(ctx context.Context, db *sql.DB) error {
	return RunFS(ctx, db, migrations.FS)
}

// RunFS giống Run nhưng nhận FS tuỳ ý (dùng cho test).
func RunFS(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("migrate: tạo locker: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys,
		goose.WithTableName(VersionTable),
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return fmt.Errorf("migrate: tạo provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}
