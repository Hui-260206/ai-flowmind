package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultHTTPAddr = ":8080"

// Config contains the configuration needed to start the minimal HTTP server.
type Config struct {
	HTTPAddr string
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

	return Config{HTTPAddr: addr}, nil
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
