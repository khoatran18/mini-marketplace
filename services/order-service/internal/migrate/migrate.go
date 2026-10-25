// Package migrate chạy các migration goose của order-service (thay cho GORM AutoMigrate).
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	"order-service/migrations"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// VersionTable là bảng version riêng của service (các service dùng chung một DB server không đè nhau).
const VersionTable = "goose_db_version_order"

// Run áp dụng mọi migration chưa chạy. Idempotent; dùng khoá phiên Postgres để chỉ một replica chạy tại một thời điểm.
// Baseline dùng IF NOT EXISTS nên cũng chạy an toàn trên DB cũ do AutoMigrate tạo.
func Run(ctx context.Context, db *sql.DB) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("goose locker: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS,
		goose.WithTableName(VersionTable), goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
