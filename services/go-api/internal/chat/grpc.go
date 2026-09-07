package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	pb "ai-flowmind/services/go-api/internal/grpcclient/pb"
	"ai-flowmind/services/go-api/internal/metrics"
	"ai-flowmind/services/go-api/internal/model"
	"ai-flowmind/services/go-api/internal/requestid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultMaxOutputTokens int32   = 1024
	defaultTemperature     float64 = 0.7
)

// GRPCChatClient 是 gRPC 完成器依赖的最小客户端边界。
type GRPCChatClient interface {
	Complete(context.Context, *pb.CompleteRequest) (*pb.CompleteResponse, error)
}

// GRPCCompleter 把领域补全请求映射为 Python AI 服务的 gRPC 调用。
type GRPCCompleter struct {
	client  GRPCChatClient
	metrics metrics.ChatMetrics
}

func NewGRPCCompleter(client GRPCChatClient, configured ...metrics.ChatMetrics) (*GRPCCompleter, error) {
	if client == nil {
		return nil, fmt.Errorf("grpc chat client: %w", ErrInvalidArgument)
	}
	collector := metrics.Noop()
	if len(configured) > 0 && configured[0] != nil {
		collector = configured[0]
	}
	return &GRPCCompleter{client: client, metrics: collector}, nil
}

func (c *GRPCCompleter) Complete(ctx context.Context, input CompletionRequest) (result CompletionResult, retErr error) {
	started := time.Now()
	defer func() {
		outcome := "success"
		if retErr != nil {
			switch {
			case errors.Is(retErr, ErrAITimeout):
				outcome = "timeout"
				c.metrics.IncAITimeout()
			case errors.Is(retErr, ErrAIUnavailable):
				outcome = "unavailable"
			case errors.Is(retErr, ErrAIProvider):
				outcome = "provider_error"
			default:
				outcome = "error"
			}
		}
		c.metrics.ObserveGRPC(outcome, time.Since(started))
	}()
	response, err := c.client.Complete(ctx, &pb.CompleteRequest{
		Context: &pb.RequestContext{
			RequestId:      requestid.FromContext(ctx),
			ConversationId: input.SessionID,
			OwnerKey:       input.OwnerKey,
			ModelProfile:   input.ModelProfile,
		},
		Messages:        toProtoMessages(input.Messages),
		MaxOutputTokens: defaultMaxOutputTokens,
		Temperature:     defaultTemperature,
	})
	if err != nil {
		return CompletionResult{}, mapGRPCError(err)
	}
	content, err := assistantText(response.GetMessage())
	if err != nil {
		return CompletionResult{}, err
	}
	usage := response.GetUsage()
	return CompletionResult{
		Content: content, ModelName: response.GetModelName(),
		PromptTokens: int(usage.GetPromptTokens()), CompletionTokens: int(usage.GetCompletionTokens()),
	}, nil
}

func toProtoMessages(messages []CompletionMessage) []*pb.ChatMessage {
	result := make([]*pb.ChatMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, &pb.ChatMessage{
			Id: message.ID, Role: toProtoRole(message.Role), Status: pb.MessageStatus_MESSAGE_STATUS_COMPLETED,
			Content: []*pb.ContentPart{{Part: &pb.ContentPart_Text{Text: &pb.TextPart{Text: message.Content}}}},
		})
	}
	return result
}

func toProtoRole(role model.MessageRole) pb.MessageRole {
	return map[model.MessageRole]pb.MessageRole{
		model.RoleSystem: pb.MessageRole_MESSAGE_ROLE_SYSTEM, model.RoleUser: pb.MessageRole_MESSAGE_ROLE_USER,
		model.RoleAssistant: pb.MessageRole_MESSAGE_ROLE_ASSISTANT, model.RoleTool: pb.MessageRole_MESSAGE_ROLE_TOOL,
	}[role]
}

func assistantText(message *pb.ChatMessage) (string, error) {
	if message.GetRole() != pb.MessageRole_MESSAGE_ROLE_ASSISTANT || message.GetStatus() != pb.MessageStatus_MESSAGE_STATUS_COMPLETED {
		return "", fmt.Errorf("invalid AI response: %w", ErrAIProvider)
	}
	var content strings.Builder
	for _, part := range message.GetContent() {
		content.WriteString(part.GetText().GetText())
	}
	if content.Len() == 0 {
		return "", fmt.Errorf("empty AI response: %w", ErrAIProvider)
	}
	return content.String(), nil
}

func mapGRPCError(err error) error {
	if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
		return fmt.Errorf("%w: %v", ErrAIUnavailable, err)
	}
	switch status.Code(err) {
	case codes.DeadlineExceeded:
		return fmt.Errorf("%w: %v", ErrAITimeout, err)
	case codes.Unavailable:
		return fmt.Errorf("%w: %v", ErrAIUnavailable, err)
	case codes.FailedPrecondition, codes.Internal:
		return fmt.Errorf("%w: %v", ErrAIProvider, err)
	default:
		return fmt.Errorf("%w: %v", ErrAICompletion, err)
	}
}
