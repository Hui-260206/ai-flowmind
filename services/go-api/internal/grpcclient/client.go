// Package grpcclient owns the gRPC connection to the Python AI service.
package grpcclient

import (
	"context"
	"fmt"
	"time"

	"ai-flowmind/services/go-api/internal/config"
	pb "ai-flowmind/services/go-api/internal/grpcclient/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the gRPC connection and the ChatService stub.
type Client struct {
	conn    *grpc.ClientConn
	chat    pb.ChatServiceClient
	timeout time.Duration
}

// Open creates a lazy gRPC client. It does not dial eagerly; the connection is
// established on the first readiness check or RPC.
func Open(cfg config.GRPCConfig) (*Client, error) {
	conn, err := grpc.NewClient(cfg.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("create grpc client: %w", err)
	}
	return &Client{
		conn:    conn,
		chat:    pb.NewChatServiceClient(conn),
		timeout: cfg.Timeout,
	}, nil
}

// Check reports whether the AI gRPC connection is ready, bounded by the client
// timeout so /readyz cannot block indefinitely.
func (c *Client) Check(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	c.conn.Connect()
	for {
		state := c.conn.GetState()
		switch state {
		case connectivity.Ready:
			return nil
		case connectivity.TransientFailure, connectivity.Shutdown:
			return fmt.Errorf("ai grpc connection is %s", state)
		}
		if !c.conn.WaitForStateChange(checkCtx, state) {
			return fmt.Errorf("ai grpc not ready: %w", checkCtx.Err())
		}
	}
}

// Complete performs a single chat completion request against the AI service.
func (c *Client) Complete(ctx context.Context, req *pb.CompleteRequest) (*pb.CompleteResponse, error) {
	return c.chat.Complete(ctx, req)
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}
