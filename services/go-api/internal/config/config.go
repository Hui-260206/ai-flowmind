package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultHTTPAddr = ":8080"

const (
	defaultShutdownTimeout   = 10 * time.Second
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 15 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultGRPCTimeout       = 15 * time.Second
)

// Config 保存 API 及其依赖共享的配置。
type Config struct {
	Environment string
	Log         LogConfig
	HTTP        HTTPConfig
	MySQL       MySQLConfig
	Redis       RedisConfig
	GRPC        GRPCConfig
}

type LogConfig struct {
	Level string
}

type HTTPConfig struct {
	Addr              string
	ShutdownTimeout   time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

type MySQLConfig struct {
	Host            string
	Port            int
	Database        string
	User            string
	Password        string
	Timezone        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type GRPCConfig struct {
	Addr    string
	Timeout time.Duration
}

// Load 从环境变量读取服务配置。
//
// GO_HTTP_ADDR 可选，默认 :8080；显式传空值会被拒绝，避免本地 .env 里写错时
// 悄悄改变监听地址。
func Load() (Config, error) {
	addr, ok := os.LookupEnv("GO_HTTP_ADDR")
	if !ok {
		addr = defaultHTTPAddr
	}
	if strings.TrimSpace(addr) == "" {
		return Config{}, fmt.Errorf("GO_HTTP_ADDR must not be empty")
	}

	if err := validateHTTPAddr(addr); err != nil {
		return Config{}, fmt.Errorf("invalid GO_HTTP_ADDR %q: %w", addr, err)
	}

	shutdownTimeout, err := loadDuration("GO_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	readHeaderTimeout, err := loadDuration("GO_READ_HEADER_TIMEOUT", defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := loadDuration("GO_READ_TIMEOUT", defaultReadTimeout)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := loadDuration("GO_WRITE_TIMEOUT", defaultWriteTimeout)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := loadDuration("GO_IDLE_TIMEOUT", defaultIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	mysql, err := loadMySQLConfig()
	if err != nil {
		return Config{}, err
	}
	redis, err := loadRedisConfig()
	if err != nil {
		return Config{}, err
	}
	grpcCfg, err := loadGRPCConfig()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment: envOrDefault("GO_ENV", "local"),
		Log:         LogConfig{Level: envOrDefault("GO_LOG_LEVEL", "info")},
		HTTP: HTTPConfig{
			Addr:              addr,
			ShutdownTimeout:   shutdownTimeout,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
		MySQL: mysql,
		Redis: redis,
		GRPC:  grpcCfg,
	}, nil
}

func loadMySQLConfig() (MySQLConfig, error) {
	host := requiredEnv("MYSQL_HOST")
	database := requiredEnv("MYSQL_DATABASE")
	user := requiredEnv("MYSQL_USER")
	password := requiredEnv("MYSQL_PASSWORD")
	if host == "" {
		host = "127.0.0.1"
	}
	if database == "" {
		return MySQLConfig{}, fmt.Errorf("MYSQL_DATABASE must not be empty")
	}
	if user == "" {
		return MySQLConfig{}, fmt.Errorf("MYSQL_USER must not be empty")
	}
	if password == "" {
		return MySQLConfig{}, fmt.Errorf("MYSQL_PASSWORD must not be empty")
	}
	port, err := loadInt("MYSQL_PORT", 3306, 1, 65535)
	if err != nil {
		return MySQLConfig{}, err
	}
	maxOpen, err := loadInt("MYSQL_MAX_OPEN_CONNS", 10, 1, 1000)
	if err != nil {
		return MySQLConfig{}, err
	}
	maxIdle, err := loadInt("MYSQL_MAX_IDLE_CONNS", 5, 0, 1000)
	if err != nil {
		return MySQLConfig{}, err
	}
	if maxIdle > maxOpen {
		return MySQLConfig{}, fmt.Errorf("MYSQL_MAX_IDLE_CONNS must not exceed MYSQL_MAX_OPEN_CONNS")
	}
	lifetime, err := loadDuration("MYSQL_CONN_MAX_LIFETIME", 30*time.Minute)
	if err != nil {
		return MySQLConfig{}, err
	}
	idleTime, err := loadDuration("MYSQL_CONN_MAX_IDLE_TIME", 10*time.Minute)
	if err != nil {
		return MySQLConfig{}, err
	}
	return MySQLConfig{
		Host: host, Port: port,
		Database: database, User: user, Password: password,
		Timezone: envOrDefault("MYSQL_TIMEZONE", "Asia/Shanghai"), MaxOpenConns: maxOpen, MaxIdleConns: maxIdle,
		ConnMaxLifetime: lifetime, ConnMaxIdleTime: idleTime,
	}, nil
}

func loadGRPCConfig() (GRPCConfig, error) {
	addr := envOrDefault("AI_GRPC_ADDR", "127.0.0.1:50051")
	if addr == "" {
		return GRPCConfig{}, fmt.Errorf("AI_GRPC_ADDR must not be empty")
	}
	timeout, err := loadDuration("AI_GRPC_TIMEOUT", defaultGRPCTimeout)
	if err != nil {
		return GRPCConfig{}, err
	}
	return GRPCConfig{Addr: addr, Timeout: timeout}, nil
}

func loadRedisConfig() (RedisConfig, error) {
	addr := envOrDefault("REDIS_ADDR", "127.0.0.1:6379")
	if addr == "" {
		return RedisConfig{}, fmt.Errorf("REDIS_ADDR must not be empty")
	}
	db, err := loadInt("REDIS_DB", 0, 0, 65535)
	if err != nil {
		return RedisConfig{}, err
	}
	return RedisConfig{
		Addr:     addr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	}, nil
}

func requiredEnv(name string) string { return strings.TrimSpace(os.Getenv(name)) }

func loadInt(name string, fallback, min, max int) (int, error) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < min || parsed > max {
		return 0, fmt.Errorf("invalid %s %q: must be an integer from %d to %d", name, value, min, max)
	}
	return parsed, nil
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func loadDuration(name string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid %s %q: must be a positive duration", name, value)
	}
	return duration, nil
}

func validateHTTPAddr(addr string) error {
	if strings.ContainsAny(addr, " \t\r\n") {
		return fmt.Errorf("must not contain whitespace")
	}

	// 接受 net/http 的 ListenAndServe 支持的写法，同时提前校验端口，
	// 以便在启动时给出有用的配置错误。
	portText := addr
	if strings.HasPrefix(addr, ":") {
		portText = strings.TrimPrefix(addr, ":")
	} else {
		lastColon := strings.LastIndexByte(addr, ':')
		if lastColon < 0 {
			return fmt.Errorf("must include a port, for example :8080")
		}
		portText = addr[lastColon+1:]
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be an integer from 1 to 65535")
	}
	return nil
}
