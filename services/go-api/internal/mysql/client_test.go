package mysql

import (
	"strings"
	"testing"
	"time"

	"ai-flowmind/services/go-api/internal/config"
)

func TestBuildDSN(t *testing.T) {
	dsn, err := buildDSN(config.MySQLConfig{
		Host: "127.0.0.1", Port: 3306, Database: "flowmind_test", User: "flowmind_app", Password: "secret",
		Timezone: "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("buildDSN() error = %v", err)
	}
	for _, want := range []string{"flowmind_app:secret@tcp(127.0.0.1:3306)/flowmind_test", "charset=utf8mb4", "parseTime=true"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("DSN %q does not contain %q", dsn, want)
		}
	}
}

func TestBuildDSNRejectsInvalidTimezone(t *testing.T) {
	_, err := buildDSN(config.MySQLConfig{Host: "localhost", Port: 3306, Timezone: "invalid/timezone"})
	if err == nil {
		t.Fatal("buildDSN() expected invalid timezone error")
	}
}

func TestOpenRequiresLiveMySQL(t *testing.T) {
	_, err := Open(config.MySQLConfig{
		Host: "127.0.0.1", Port: 1, Database: "flowmind_test", User: "flowmind_app", Password: "secret",
		Timezone: "Asia/Shanghai", MaxOpenConns: 2, MaxIdleConns: 1,
		ConnMaxLifetime: time.Minute, ConnMaxIdleTime: time.Minute,
	})
	if err == nil || (!strings.Contains(err.Error(), "ping mysql") && !strings.Contains(err.Error(), "open mysql with gorm")) {
		t.Fatalf("Open() error = %v, want a clear mysql connection error", err)
	}
}
