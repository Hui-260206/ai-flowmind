//go:build integration

package redis

import (
	"context"
	"testing"
	"time"

	"ai-flowmind/services/go-api/internal/chat"
	"ai-flowmind/services/go-api/internal/config"
)

func openReliabilityAdapter(t *testing.T, limit int) *ReliabilityAdapter {
	t.Helper()
	client, err := Open(config.RedisConfig{Addr: "127.0.0.1:6379"})
	if err != nil {
		t.Skipf("skip Redis integration test: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	adapter, err := NewReliabilityAdapter(client, time.Hour, 100*time.Millisecond, limit)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestReliabilityAdapterLifecycle(t *testing.T) {
	adapter := openReliabilityAdapter(t, 1)
	ctx := context.Background()
	req := chat.IdempotencyRequest{OwnerKey: "integration-owner", SessionID: "integration-session", ClientMessageID: "integration-message", Fingerprint: "fingerprint"}
	claim, err := adapter.Claim(ctx, req)
	if err != nil || claim.State != chat.ClaimNew {
		t.Fatalf("first Claim() = %#v, %v", claim, err)
	}
	if err := adapter.AttachUserMessage(ctx, claim, "user-1"); err != nil {
		t.Fatal(err)
	}
	duplicate, err := adapter.Claim(ctx, req)
	if err != nil || duplicate.State != chat.ClaimProcessing {
		t.Fatalf("processing Claim() = %#v, %v", duplicate, err)
	}
	if err := adapter.Fail(ctx, claim, "user-1"); err != nil {
		t.Fatal(err)
	}
	resumed, err := adapter.Claim(ctx, req)
	if err != nil || resumed.State != chat.ClaimFailed || resumed.UserMessageID != "user-1" {
		t.Fatalf("failed Claim() = %#v, %v", resumed, err)
	}
	if err := adapter.Complete(ctx, resumed, "user-1", "assistant-1"); err != nil {
		t.Fatal(err)
	}
	completed, err := adapter.Claim(ctx, req)
	if err != nil || completed.State != chat.ClaimCompleted || completed.AssistantMessageID != "assistant-1" {
		t.Fatalf("completed Claim() = %#v, %v", completed, err)
	}
	mismatch, err := adapter.Claim(ctx, chat.IdempotencyRequest{OwnerKey: req.OwnerKey, SessionID: req.SessionID, ClientMessageID: req.ClientMessageID, Fingerprint: "other"})
	if err != nil || mismatch.State != chat.ClaimMismatch {
		t.Fatalf("mismatch Claim() = %#v, %v", mismatch, err)
	}
}

func TestReliabilityAdapterLockAndRateLimit(t *testing.T) {
	adapter := openReliabilityAdapter(t, 1)
	ctx := context.Background()
	lock, ok, err := adapter.AcquireSession(ctx, "lock-session")
	if err != nil || !ok {
		t.Fatalf("AcquireSession() = %#v, %v", lock, err)
	}
	if _, ok, err := adapter.AcquireSession(ctx, "lock-session"); err != nil || ok {
		t.Fatalf("second AcquireSession() = %v, %v, want false nil", ok, err)
	}
	if err := adapter.ReleaseSession(ctx, chat.SessionLock{SessionID: lock.SessionID, Token: "wrong"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := adapter.AcquireSession(ctx, "lock-session"); ok {
		t.Fatal("wrong token released replacement lock")
	}
	if err := adapter.ReleaseSession(ctx, lock); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := adapter.AcquireSession(ctx, "lock-session"); err != nil || !ok {
		t.Fatalf("release did not unlock: %v, %v", ok, err)
	}
	now := time.Now()
	allowed, err := adapter.AllowOwner(ctx, "rate-owner", now)
	if err != nil || !allowed {
		t.Fatalf("first AllowOwner = %v, %v", allowed, err)
	}
	allowed, err = adapter.AllowOwner(ctx, "rate-owner", now)
	if err != nil || allowed {
		t.Fatalf("second AllowOwner = %v, %v, want false nil", allowed, err)
	}
}
