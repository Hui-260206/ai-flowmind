package chat

import (
	"context"
	"errors"
	"testing"

	pb "ai-flowmind/services/go-api/internal/grpcclient/pb"
	"ai-flowmind/services/go-api/internal/model"
	"ai-flowmind/services/go-api/internal/requestid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcClientStub struct {
	request  *pb.CompleteRequest
	response *pb.CompleteResponse
	err      error
}

func (c *grpcClientStub) Complete(_ context.Context, request *pb.CompleteRequest) (*pb.CompleteResponse, error) {
	c.request = request
	return c.response, c.err
}

func TestGRPCCompleterMapsRequestAndResponse(t *testing.T) {
	client := &grpcClientStub{response: &pb.CompleteResponse{
		Message: &pb.ChatMessage{
			Role: pb.MessageRole_MESSAGE_ROLE_ASSISTANT, Status: pb.MessageStatus_MESSAGE_STATUS_COMPLETED,
			Content: []*pb.ContentPart{{Part: &pb.ContentPart_Text{Text: &pb.TextPart{Text: "模型回复"}}}},
		},
		ModelName: "hy3", Usage: &pb.TokenUsage{PromptTokens: 3, CompletionTokens: 5},
	}}
	completer, err := NewGRPCCompleter(client)
	if err != nil {
		t.Fatalf("NewGRPCCompleter() error = %v", err)
	}
	ctx := requestid.WithContext(context.Background(), "req-grpc-123")
	result, err := completer.Complete(ctx, CompletionRequest{
		OwnerKey: "client:test", SessionID: "session-1", ModelProfile: "default",
		Messages: []CompletionMessage{{ID: "message-1", Role: model.RoleUser, Content: "你好"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if client.request.GetContext().GetRequestId() != "req-grpc-123" || client.request.GetContext().GetConversationId() != "session-1" {
		t.Fatalf("request context = %#v", client.request.GetContext())
	}
	if len(client.request.GetMessages()) != 1 || client.request.GetMessages()[0].GetContent()[0].GetText().GetText() != "你好" {
		t.Fatalf("messages = %#v", client.request.GetMessages())
	}
	if result.Content != "模型回复" || result.ModelName != "hy3" || result.PromptTokens != 3 || result.CompletionTokens != 5 {
		t.Fatalf("result = %#v", result)
	}
}

func TestGRPCCompleterClassifiesFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "timeout", err: status.Error(codes.DeadlineExceeded, "deadline"), want: ErrAITimeout},
		{name: "unavailable", err: status.Error(codes.Unavailable, "down"), want: ErrAIUnavailable},
		{name: "provider", err: status.Error(codes.Internal, "provider"), want: ErrAIProvider},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completer, _ := NewGRPCCompleter(&grpcClientStub{err: tt.err})
			_, err := completer.Complete(context.Background(), CompletionRequest{})
			if !errors.Is(err, tt.want) {
				t.Fatalf("Complete() error = %v, want %v", err, tt.want)
			}
		})
	}
}
