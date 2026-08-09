package config

import "testing"

func TestLoadDefaultsHTTPAddr(t *testing.T) {
	t.Setenv("GO_HTTP_ADDR", "")
	// An empty environment value is intentionally invalid rather than treated
	// as an omitted value.
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error for an empty GO_HTTP_ADDR")
	}

	t.Setenv("GO_HTTP_ADDR", ":9090")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":9090")
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
