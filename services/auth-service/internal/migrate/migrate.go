// Package migrate chạy migration goose (nhúng trong ./migrations) cho auth-service.
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	"auth-service/migrations"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// TableName là bảng version riêng của service.
const TableName = "goose_db_version_auth"

// Run áp dụng mọi migration chưa chạy; khoá phiên Postgres đảm bảo chỉ 1 replica chạy.
// Baseline dùng IF NOT EXISTS nên DB cũ (do AutoMigrate tạo) vẫn lên an toàn.
func Run(ctx context.Context, db *sql.DB) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("goose locker: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS,
		goose.WithTableName(TableName), goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
