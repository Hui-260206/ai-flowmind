package redis

import (
	"testing"
	"time"

	"ai-flowmind/services/go-api/internal/chat"
)

func TestReliabilityKeysHashUntrustedParts(t *testing.T) {
	req := chat.IdempotencyRequest{OwnerKey: "owner:{evil}", SessionID: "session:*", ClientMessageID: "id: spaces", Fingerprint: "f"}
	idempotency, lock, rate := ReliabilityKeys(req, time.Date(2026, 9, 6, 8, 1, 0, 0, time.UTC))
	if idempotency == "" || lock == "" || rate == "" {
		t.Fatal("keys must not be empty")
	}
	if idempotency == "idempotency:owner:{evil}:session:*:id: spaces" || lock == "lock:conversation:session:*" {
		t.Fatalf("keys leaked untrusted parts: %q %q", idempotency, lock)
	}
}

func TestReliabilityAdapterRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewReliabilityAdapter(nil, time.Hour, time.Second, 1); err == nil {
		t.Fatal("expected nil client error")
	}
	client := &Client{}
	if _, err := NewReliabilityAdapter(client, 0, time.Second, 1); err == nil {
		t.Fatal("expected invalid TTL error")
	}
}

func TestReliabilityAdapterRejectsNilUnderlyingClient(t *testing.T) {
	client := &Client{client: nil}
	adapter, err := NewReliabilityAdapter(client, time.Hour, time.Second, 1)
	if err == nil || adapter != nil {
		t.Fatalf("NewReliabilityAdapter() = %#v, %v, want validation error", adapter, err)
	}
}
