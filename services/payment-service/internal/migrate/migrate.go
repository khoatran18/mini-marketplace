// Package migrate chạy các migration goose (nhúng trong services/payment-service/migrations).
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"payment-service/migrations"
)

// VersionTable là bảng version riêng của service này.
const VersionTable = "goose_db_version_payment"

// Run áp dụng mọi migration chưa chạy. Dùng khoá phiên Postgres để chỉ một replica chạy tại một thời điểm.
// Baseline dùng IF NOT EXISTS nên cũng an toàn với DB cũ do GORM AutoMigrate tạo trước đây.
func Run(ctx context.Context, db *sql.DB) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("migrate: locker: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS,
		goose.WithTableName(VersionTable), goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("migrate: provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}
