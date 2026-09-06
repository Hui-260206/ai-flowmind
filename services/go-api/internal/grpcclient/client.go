// Package grpcclient 负责到 Python AI 服务的 gRPC 连接。
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
	"google.golang.org/grpc/keepalive"
)

// Client 封装 gRPC 连接与 ChatService stub。
type Client struct {
	conn    *grpc.ClientConn
	chat    pb.ChatServiceClient
	timeout time.Duration
}

// Open 创建懒连接的 gRPC 客户端：不主动拨号，连接在首次就绪检查或 RPC 时建立。
func Open(cfg config.GRPCConfig) (*Client, error) {
	// 单元测试和嵌入式调用可直接构造最小配置；生产配置由 config.Load 校验。
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.KeepaliveTime <= 0 {
		cfg.KeepaliveTime = 30 * time.Second
	}
	if cfg.KeepaliveTimeout <= 0 {
		cfg.KeepaliveTimeout = 10 * time.Second
	}
	if cfg.MaxMessageBytes <= 0 {
		cfg.MaxMessageBytes = 1 << 20
	}
	conn, err := grpc.NewClient(
		cfg.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: cfg.KeepaliveTime, Timeout: cfg.KeepaliveTimeout,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(cfg.MaxMessageBytes),
			grpc.MaxCallRecvMsgSize(cfg.MaxMessageBytes),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create grpc client: %w", err)
	}
	return &Client{
		conn:    conn,
		chat:    pb.NewChatServiceClient(conn),
		timeout: cfg.Timeout,
	}, nil
}

// Check 判断 AI gRPC 连接是否就绪，用客户端超时限制等待时长，避免 /readyz 无限阻塞。
// TransientFailure 视为可恢复状态：探针会一直等到连接进入 Ready 或超时为止，因此
// AI 服务重启后能正确反映为就绪，不会出现假阴性。
func (c *Client) Check(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	c.conn.Connect()
	for {
		state := c.conn.GetState()
		switch state {
		case connectivity.Ready:
			return nil
		case connectivity.Shutdown:
			return fmt.Errorf("ai grpc connection is %s", state)
		}
		// Idle / Connecting / TransientFailure 都表示「尚未就绪但可能恢复」，
		// 继续等待状态变化；只有超时或真正 Shutdown 才返回失败。这样 AI 服务
		// 重启后 /readyz 能在状态切换时自然恢复，而不是把 TransientFailure
		// 当作终态立即误报。
		if !c.conn.WaitForStateChange(checkCtx, state) {
			return fmt.Errorf("ai grpc not ready: %w", checkCtx.Err())
		}
	}
}

// Complete 对 AI 服务发起一次聊天补全请求。
func (c *Client) Complete(ctx context.Context, req *pb.CompleteRequest) (*pb.CompleteResponse, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.chat.Complete(rpcCtx, req)
}

// Close 关闭底层 gRPC 连接。
func (c *Client) Close() error {
	return c.conn.Close()
}
