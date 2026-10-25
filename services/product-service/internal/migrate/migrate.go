// Package migrate chạy các migration goose (thư mục migrations/) của product-service.
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"

	"product-service/migrations"
)

// VersionTable là bảng version riêng của service (DB dùng chung server với service khác).
const VersionTable = "goose_db_version_product"

// Run áp dụng mọi migration chưa chạy. Idempotent; dùng session lock của Postgres để chỉ một replica
// migrate tại một thời điểm. DB cũ do AutoMigrate tạo (chưa có bảng version) vẫn lên được vì baseline
// dùng IF NOT EXISTS.
func Run(ctx context.Context, db *sql.DB) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("goose locker: %w", err)
	}
	p, err := goose.NewProvider(database.DialectPostgres, db, migrations.FS,
		goose.WithTableName(VersionTable), goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
