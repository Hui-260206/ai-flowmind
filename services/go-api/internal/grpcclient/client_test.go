package grpcclient

import (
	"context"
	"net"
	"testing"
	"time"

	"ai-flowmind/services/go-api/internal/config"
	pb "ai-flowmind/services/go-api/internal/grpcclient/pb"

	"google.golang.org/grpc"
)

type fakeChatServer struct {
	pb.UnimplementedChatServiceServer
}

func (fakeChatServer) Complete(_ context.Context, _ *pb.CompleteRequest) (*pb.CompleteResponse, error) {
	return &pb.CompleteResponse{ModelName: "fake"}, nil
}

func startServer(t *testing.T) (*grpc.Server, net.Listener, string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterChatServiceServer(srv, fakeChatServer{})
	go func() { _ = srv.Serve(lis) }()
	return srv, lis, lis.Addr().String()
}

func TestCheckReadyWhenServerUp(t *testing.T) {
	srv, lis, addr := startServer(t)
	defer srv.Stop()
	defer lis.Close()

	client, err := Open(config.GRPCConfig{Addr: addr, Timeout: time.Second})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()

	if err := client.Check(context.Background()); err != nil {
		t.Fatalf("Check() = %v, want nil", err)
	}
}

func TestCheckFailsWhenServerDown(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()
	_ = lis.Close() // 该端口现在没有任何服务在监听

	client, err := Open(config.GRPCConfig{Addr: addr, Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()

	if err := client.Check(context.Background()); err == nil {
		t.Fatal("Check() = nil, want error when server is down")
	}
}

func TestCompleteCallsServer(t *testing.T) {
	srv, lis, addr := startServer(t)
	defer srv.Stop()
	defer lis.Close()

	client, err := Open(config.GRPCConfig{Addr: addr, Timeout: time.Second})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()

	resp, err := client.Complete(context.Background(), &pb.CompleteRequest{})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp.GetModelName() != "fake" {
		t.Fatalf("Complete() model_name = %q, want fake", resp.GetModelName())
	}
}
