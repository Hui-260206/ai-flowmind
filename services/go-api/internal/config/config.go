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
)

// Config contains configuration shared by the API and its dependencies.
type Config struct {
	Environment string
	Log         LogConfig
	HTTP        HTTPConfig
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

// Load reads the server configuration from the environment.
//
// GO_HTTP_ADDR is optional and defaults to :8080. An explicit empty value is
// rejected so a typo in a local .env file cannot silently change the binding.
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
	}, nil
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

	// Accept the forms supported by net/http's ListenAndServe, while checking
	// the port early to produce a useful configuration error at startup.
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
