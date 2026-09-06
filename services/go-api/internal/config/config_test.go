package config

import (
	"testing"
	"time"
)

func setMySQLTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("MYSQL_DATABASE", "flowmind_test")
	t.Setenv("MYSQL_USER", "flowmind_test")
	t.Setenv("MYSQL_PASSWORD", "test-password")
}

func TestLoadDefaultsHTTPAddr(t *testing.T) {
	setMySQLTestEnv(t)
	t.Setenv("GO_HTTP_ADDR", "")
	// 空环境变量值被有意视为非法，而不是当作「未设置」。
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error for an empty GO_HTTP_ADDR")
	}

	t.Setenv("GO_HTTP_ADDR", ":9090")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Addr != ":9090" {
		t.Fatalf("HTTP.Addr = %q, want %q", cfg.HTTP.Addr, ":9090")
	}
	if cfg.HTTP.ShutdownTimeout != 10*time.Second {
		t.Fatalf("HTTP.ShutdownTimeout = %s, want 10s", cfg.HTTP.ShutdownTimeout)
	}
}

func TestLoadDurations(t *testing.T) {
	setMySQLTestEnv(t)
	t.Setenv("GO_HTTP_ADDR", ":8080")
	t.Setenv("GO_SHUTDOWN_TIMEOUT", "2s")
	t.Setenv("GO_READ_HEADER_TIMEOUT", "300ms")
	t.Setenv("GO_READ_TIMEOUT", "4s")
	t.Setenv("GO_WRITE_TIMEOUT", "5s")
	t.Setenv("GO_IDLE_TIMEOUT", "1m")
	t.Setenv("AI_PROVIDER_TIMEOUT", "3")
	t.Setenv("AI_GRPC_TIMEOUT", "4s")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.ShutdownTimeout != 2*time.Second || cfg.HTTP.ReadHeaderTimeout != 300*time.Millisecond || cfg.HTTP.IdleTimeout != time.Minute {
		t.Fatalf("unexpected durations: %+v", cfg)
	}
}

func TestLoadRedisConfig(t *testing.T) {
	setMySQLTestEnv(t)
	t.Setenv("GO_HTTP_ADDR", ":8080")
	t.Setenv("REDIS_ADDR", "redis.example.test:6380")
	t.Setenv("REDIS_PASSWORD", "redis-test-password")
	t.Setenv("REDIS_DB", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Redis.Addr != "redis.example.test:6380" || cfg.Redis.Password != "redis-test-password" || cfg.Redis.DB != 3 {
		t.Fatalf("unexpected Redis config: %+v", cfg.Redis)
	}
}

func TestLoadRejectsInvalidRedisDB(t *testing.T) {
	setMySQLTestEnv(t)
	t.Setenv("GO_HTTP_ADDR", ":8080")
	t.Setenv("REDIS_DB", "-1")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error for a negative Redis database number")
	}
}

func TestLoadRejectsInvalidHTTPAddr(t *testing.T) {
	for _, addr := range []string{"8080", ":0", ":65536", ":http", ""} {
		t.Run(addr, func(t *testing.T) {
			t.Setenv("GO_HTTP_ADDR", addr)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() expected an error for %q", addr)
			}
		})
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("GO_SHUTDOWN_TIMEOUT", "0s")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error for a non-positive timeout")
	}
}

func TestLoadGRPCConfig(t *testing.T) {
	setMySQLTestEnv(t)
	t.Setenv("GO_HTTP_ADDR", ":8080")
	t.Setenv("AI_GRPC_ADDR", "ai-service:50051")
	t.Setenv("AI_GRPC_TIMEOUT", "30s")
	t.Setenv("GO_WRITE_TIMEOUT", "31s")
	t.Setenv("AI_GRPC_KEEPALIVE_TIME", "45s")
	t.Setenv("AI_GRPC_KEEPALIVE_TIMEOUT", "6s")
	t.Setenv("AI_GRPC_MAX_MESSAGE_BYTES", "2097152")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GRPC.Addr != "ai-service:50051" || cfg.GRPC.Timeout != 30*time.Second || cfg.GRPC.KeepaliveTime != 45*time.Second || cfg.GRPC.KeepaliveTimeout != 6*time.Second || cfg.GRPC.MaxMessageBytes != 2097152 {
		t.Fatalf("unexpected gRPC config: %+v", cfg.GRPC)
	}
}

func TestLoadGRPCDefaults(t *testing.T) {
	setMySQLTestEnv(t)
	t.Setenv("GO_HTTP_ADDR", ":8080")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GRPC.Addr != "127.0.0.1:50051" || cfg.GRPC.Timeout != 14*time.Second || cfg.GRPC.MaxMessageBytes != 1<<20 {
		t.Fatalf("unexpected gRPC defaults: %+v", cfg.GRPC)
	}
}

func TestLoadRejectsInvalidAITimeoutBudget(t *testing.T) {
	setMySQLTestEnv(t)
	t.Setenv("GO_HTTP_ADDR", ":8080")
	t.Setenv("AI_PROVIDER_TIMEOUT", "5")
	t.Setenv("AI_GRPC_TIMEOUT", "5s")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error when provider timeout reaches gRPC timeout")
	}

	t.Setenv("AI_PROVIDER_TIMEOUT", "3")
	t.Setenv("AI_GRPC_TIMEOUT", "5s")
	t.Setenv("GO_WRITE_TIMEOUT", "5s")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error when gRPC timeout reaches write timeout")
	}
}
