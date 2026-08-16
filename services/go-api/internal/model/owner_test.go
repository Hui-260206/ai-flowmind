package model

import "testing"

func TestOwnerKey(t *testing.T) {
	if got := OwnerKey("abc-123"); got != "client:abc-123" {
		t.Fatalf("OwnerKey() = %q, want %q", got, "client:abc-123")
	}
}
