package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

func TestCollectorExposesOnlyBoundedLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector := New(registry)
	collector.ObserveChatRequest("success", "", 200, 12*time.Millisecond)
	collector.ObserveChatRequest("timeout", "AI_TIMEOUT", 504, 8*time.Millisecond)
	collector.ObserveGRPC("timeout", 7*time.Millisecond)
	collector.IncAITimeout()
	collector.IncMessagePersistFailure("assistant")
	collector.IncRedisLockFailure("acquire")

	var output strings.Builder
	metrics, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	for _, metric := range metrics {
		if _, err := expfmt.MetricFamilyToText(&output, metric); err != nil {
			t.Fatal(err)
		}
	}
	text := output.String()
	for _, name := range []string{"chat_request_total", "chat_request_failed_total", "chat_request_latency", "ai_grpc_latency", "ai_timeout_total", "message_persist_failed_total", "redis_lock_failed_total"} {
		if !strings.Contains(text, name) {
			t.Fatalf("missing metric %q in %s", name, text)
		}
	}
	for _, forbidden := range []string{"request_id", "session_id", "message_id", "owner_key", "content", "provider_url"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("forbidden high-cardinality label %q in %s", forbidden, text)
		}
	}
	for _, expected := range []string{`message_persist_failed_total{stage="assistant"} 1`, `redis_lock_failed_total{stage="acquire"} 1`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing expected metric sample %q in %s", expected, text)
		}
	}
}
