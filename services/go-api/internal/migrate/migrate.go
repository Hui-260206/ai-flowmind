// Package migrate 内嵌 migrations 目录下的 SQL 文件，并在启动时按版本执行。
package migrate

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	gomigrate "github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Up 把 migrations 目录中尚未执行的版本依次应用到 db。
//
// golang-migrate 会在目标库维护一张 schema_migrations 表记录已执行版本，
// 因此重复调用是安全的（已是最新时返回 nil，不会重复建表）。
func Up(db *sql.DB) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}
	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		return fmt.Errorf("init mysql migration driver: %w", err)
	}
	m, err := gomigrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, gomigrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
