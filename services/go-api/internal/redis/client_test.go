package redis

import (
	"testing"

	"ai-flowmind/services/go-api/internal/config"
)

func TestOpenRejectsEmptyAddress(t *testing.T) {
	_, err := Open(config.RedisConfig{})
	if err == nil {
		t.Fatal("Open() expected an error for an empty address")
	}
}
