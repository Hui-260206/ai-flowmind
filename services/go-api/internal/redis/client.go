package redis

import (
	"context"
	"fmt"

	"ai-flowmind/services/go-api/internal/config"
	redisgo "github.com/redis/go-redis/v9"
)

// Client owns the Redis connection pool used by the API.
type Client struct {
	client *redisgo.Client
}

func Open(cfg config.RedisConfig) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis address must not be empty")
	}

	client := redisgo.NewClient(&redisgo.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Client{client: client}, nil
}

func (c *Client) Client() *redisgo.Client {
	return c.client
}

func (c *Client) Check(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Client) Close() error {
	return c.client.Close()
}
