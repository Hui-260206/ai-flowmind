package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-flowmind/services/go-api/internal/config"
)

func testServer() *Server {
	return New(config.HTTPConfig{Addr: ":8080"}, nil, Dependencies{})
}

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()
	testServer().Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK || res.Body.String() != "ok\n" {
		t.Fatalf("response = %d %q, want 200 %q", res.Code, res.Body.String(), "ok\n")
	}
}

func TestRequestIDIsPropagatedAndGenerated(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "provided", id: "client-request-123"},
		{name: "generated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			if tt.id != "" {
				req.Header.Set("X-Request-ID", tt.id)
			}
			res := httptest.NewRecorder()
			testServer().Handler().ServeHTTP(res, req)
			if got := res.Header().Get("X-Request-ID"); got == "" || (tt.id != "" && got != tt.id) {
				t.Fatalf("X-Request-ID = %q, want %q", got, tt.id)
			}
		})
	}
}

func TestInvalidRequestIDIsReplaced(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", strings.Repeat("x", 129))
	res := httptest.NewRecorder()
	testServer().Handler().ServeHTTP(res, req)
	if got := res.Header().Get("X-Request-ID"); got == "" || len(got) == 129 {
		t.Fatalf("invalid request ID was not replaced: %q", got)
	}
}

func TestMissingRouteUsesContractErrorEnvelope(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	res := httptest.NewRecorder()
	testServer().Handler().ServeHTTP(res, req)

	if res.Code != http.StatusNotFound || !strings.Contains(res.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("response = %d, content type = %q", res.Code, res.Header().Get("Content-Type"))
	}
	var body struct {
		RequestID string `json:"request_id"`
		Error     struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.RequestID == "" || body.Error.RequestID != "" || body.Error.Code != "NOT_FOUND" || body.Error.Message == "" {
		t.Fatalf("unexpected error body: %s", res.Body.String())
	}
}
