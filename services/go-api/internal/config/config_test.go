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
	t.Setenv("REDIS_IDEMPOTENCY_TTL", "36h")
	t.Setenv("REDIS_SESSION_LOCK_TTL", "45s")
	t.Setenv("REDIS_OWNER_RATE_LIMIT_PER_MINUTE", "25")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Redis.Addr != "redis.example.test:6380" || cfg.Redis.Password != "redis-test-password" || cfg.Redis.DB != 3 ||
		cfg.Redis.IdempotencyTTL != 36*time.Hour || cfg.Redis.SessionLockTTL != 45*time.Second || cfg.Redis.OwnerRateLimitPerMinute != 25 {
		t.Fatalf("unexpected Redis config: %+v", cfg.Redis)
	}
}

func TestLoadRedisReliabilityDefaults(t *testing.T) {
	setMySQLTestEnv(t)
	t.Setenv("GO_HTTP_ADDR", ":8080")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Redis.IdempotencyTTL != 48*time.Hour || cfg.Redis.SessionLockTTL != 30*time.Second || cfg.Redis.OwnerRateLimitPerMinute != 20 {
		t.Fatalf("unexpected Redis reliability defaults: %+v", cfg.Redis)
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

func TestLoadRejectsInvalidRedisReliabilityConfig(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "non-positive idempotency TTL", key: "REDIS_IDEMPOTENCY_TTL", value: "0s"},
		{name: "non-positive lock TTL", key: "REDIS_SESSION_LOCK_TTL", value: "0s"},
		{name: "zero owner limit", key: "REDIS_OWNER_RATE_LIMIT_PER_MINUTE", value: "0"},
		{name: "negative owner limit", key: "REDIS_OWNER_RATE_LIMIT_PER_MINUTE", value: "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMySQLTestEnv(t)
			t.Setenv("GO_HTTP_ADDR", ":8080")
			t.Setenv(tt.key, tt.value)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() expected an error for %s=%q", tt.key, tt.value)
			}
		})
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
	t.Setenv("REDIS_SESSION_LOCK_TTL", "32s")
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

func TestLoadRejectsSessionLockThatCannotCoverSynchronousBudget(t *testing.T) {
	setMySQLTestEnv(t)
	t.Setenv("GO_HTTP_ADDR", ":8080")
	t.Setenv("REDIS_SESSION_LOCK_TTL", "15s")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error when session lock TTL reaches HTTP write timeout")
	}

	t.Setenv("GO_WRITE_TIMEOUT", "22s")
	t.Setenv("AI_GRPC_TIMEOUT", "21s")
	t.Setenv("REDIS_SESSION_LOCK_TTL", "21s")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error when session lock TTL reaches gRPC timeout")
	}
}
