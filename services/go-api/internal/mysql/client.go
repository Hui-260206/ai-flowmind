package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"time"

	"ai-flowmind/services/go-api/internal/config"
	sqlmysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Client struct {
	db    *gorm.DB
	sqlDB *sql.DB
}

func Open(cfg config.MySQLConfig) (*Client, error) {
	dsn, err := buildDSN(cfg)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open mysql with gorm: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get mysql connection pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return &Client{db: db, sqlDB: sqlDB}, nil
}
func (c *Client) DB() *gorm.DB                    { return c.db }
func (c *Client) SQLDB() *sql.DB                  { return c.sqlDB }
func (c *Client) Check(ctx context.Context) error { return c.sqlDB.PingContext(ctx) }
func (c *Client) Close() error                    { return c.sqlDB.Close() }

func buildDSN(cfg config.MySQLConfig) (string, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return "", fmt.Errorf("invalid MYSQL_TIMEZONE %q: %w", cfg.Timezone, err)
	}
	return (&sqlmysql.Config{User: cfg.User, Passwd: cfg.Password, Net: "tcp", Addr: net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), DBName: cfg.Database, Loc: loc, ParseTime: true, Params: map[string]string{"charset": "utf8mb4"}}).FormatDSN(), nil
}
