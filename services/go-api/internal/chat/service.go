// Package chat 包含 REST 聊天用例的应用层实现。
//
// 它协调 Repository 与 AI 补全调用，但不依赖 Gin 或某种具体 AI 传输方式。
// HTTP handler 与未来的 gRPC Adapter 都位于此包之外。
package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"ai-flowmind/services/go-api/internal/model"
	"ai-flowmind/services/go-api/internal/repository"
)

const (
	DefaultModelProfile = "default"
	InitialSessionTitle = "新会话"
	maxMessageRunes     = 32000
	maxClientMessageID  = 128
	maxModelProfile     = 64
	contextTokenBudget  = 32000
)

var (
	// ErrInvalidArgument 表示聊天用例拒绝了输入参数。
	ErrInvalidArgument = errors.New("invalid chat argument")
	// ErrSessionNotFound 有意同时表示会话不存在和会话不属于当前调用者。
	ErrSessionNotFound = errors.New("chat session not found")
	// ErrDuplicateRequest 表示同一会话内重复使用了 client_message_id。
	ErrDuplicateRequest = errors.New("duplicate chat request")
	// ErrAICompletion 表示已配置的 AI 完成器调用失败。
	ErrAICompletion = errors.New("ai completion failed")
)

// Completer 是生成助手消息的可替换边界。
// 阶段 3 注入进程内 Fake 实现；阶段 5 注入 gRPC Adapter，二者都不应改变
// 聊天用例或 HTTP handler。
type Completer interface {
	Complete(ctx context.Context, request CompletionRequest) (CompletionResult, error)
}

// CompletionRequest 是传给 Completer 的、与具体模型无关的输入。
type CompletionRequest struct {
	OwnerKey     string
	SessionID    string
	ModelProfile string
	Messages     []CompletionMessage
}

// CompletionMessage 是作为 AI 上下文传入的一条已完成文本消息。
type CompletionMessage struct {
	ID      string
	Role    model.MessageRole
	Content string
}

// CompletionResult 是 Completer 返回的、与具体模型无关的助手结果。
type CompletionResult struct {
	Content          string
	ModelName        string
	PromptTokens     int
	CompletionTokens int
}

// CreateSessionInput 描述创建匿名会话所需的数据。
type CreateSessionInput struct {
	ClientID     string
	ModelProfile string
}

// ListSessionsInput 标识要查询其会话列表的匿名客户端。
type ListSessionsInput struct {
	ClientID string
}

// SessionInput 标识读取或删除操作所作用的、归当前客户端所有的会话。
type SessionInput struct {
	ClientID  string
	SessionID string
}

// SendMessageInput 描述一次同步文本补全请求。
type SendMessageInput struct {
	ClientID        string
	SessionID       string
	Content         string
	ClientMessageID string
	ModelProfile    string
}

// CreateSessionResult 包含 CreateSession 返回的已持久化会话。
type CreateSessionResult struct {
	Session model.Session
}

// ListSessionsResult 包含按 Repository 契约排序的会话。
type ListSessionsResult struct {
	Sessions []model.Session
}

// ListMessagesResult 包含按消息序号升序排列的消息。
type ListMessagesResult struct {
	Messages []model.Message
}

// SendMessageResult 包含已持久化的用户消息和助手消息。
type SendMessageResult struct {
	UserMessage      model.Message
	AssistantMessage model.Message
}

// Dependencies 是聊天应用服务所需的协作依赖。
// 注入 Now 和 NewID，以便用例测试能够稳定、可预测。
type Dependencies struct {
	Sessions  repository.SessionRepository
	Messages  repository.MessageRepository
	Completer Completer
	Now       func() time.Time
	NewID     func() string
}

// Service 协调聊天用例；随着阶段 3 的任务推进，方法会逐步加入。
type Service struct {
	sessions  repository.SessionRepository
	messages  repository.MessageRepository
	completer Completer
	now       func() time.Time
	newID     func() string
}

// New 仅在所有协作边界均显式提供时创建聊天应用服务。这能避免生产代码悄然
// 选用测试无法控制的时钟、ID 生成器或 AI 实现。
func New(deps Dependencies) (*Service, error) {
	switch {
	case deps.Sessions == nil:
		return nil, fmt.Errorf("chat sessions repository: %w", ErrInvalidArgument)
	case deps.Messages == nil:
		return nil, fmt.Errorf("chat messages repository: %w", ErrInvalidArgument)
	case deps.Completer == nil:
		return nil, fmt.Errorf("chat completer: %w", ErrInvalidArgument)
	case deps.Now == nil:
		return nil, fmt.Errorf("chat clock: %w", ErrInvalidArgument)
	case deps.NewID == nil:
		return nil, fmt.Errorf("chat ID generator: %w", ErrInvalidArgument)
	}

	return &Service{
		sessions:  deps.Sessions,
		messages:  deps.Messages,
		completer: deps.Completer,
		now:       deps.Now,
		newID:     deps.NewID,
	}, nil
}

// CreateSession 创建当前匿名客户端的空会话。
func (s *Service) CreateSession(ctx context.Context, input CreateSessionInput) (CreateSessionResult, error) {
	clientID, profile, err := validateSessionInput(input.ClientID, input.ModelProfile)
	if err != nil {
		return CreateSessionResult{}, err
	}
	now := s.now()
	session := model.Session{
		ID:           s.newID(),
		OwnerKey:     model.OwnerKey(clientID),
		Title:        InitialSessionTitle,
		ModelProfile: profile,
		Status:       model.SessionActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if session.ID == "" {
		return CreateSessionResult{}, fmt.Errorf("generate session ID")
	}
	if err := s.sessions.Create(ctx, &session); err != nil {
		return CreateSessionResult{}, fmt.Errorf("create session: %w", err)
	}
	return CreateSessionResult{Session: session}, nil
}

// ListSessions 返回当前匿名客户端的未删除会话。
func (s *Service) ListSessions(ctx context.Context, input ListSessionsInput) (ListSessionsResult, error) {
	clientID, err := validateClientID(input.ClientID)
	if err != nil {
		return ListSessionsResult{}, err
	}
	sessions, err := s.sessions.ListByOwner(ctx, model.OwnerKey(clientID))
	if err != nil {
		return ListSessionsResult{}, fmt.Errorf("list sessions: %w", err)
	}
	return ListSessionsResult{Sessions: sessions}, nil
}

// DeleteSession 软删除当前匿名客户端拥有的会话。
func (s *Service) DeleteSession(ctx context.Context, input SessionInput) error {
	clientID, sessionID, err := validateOwnedSessionInput(input)
	if err != nil {
		return err
	}
	if err := s.sessions.SoftDelete(ctx, model.OwnerKey(clientID), sessionID); err != nil {
		return mapRepositoryError(err, "delete session")
	}
	return nil
}

// ListMessages 先验证归属，再按 seq 升序读取会话历史。
func (s *Service) ListMessages(ctx context.Context, input SessionInput) (ListMessagesResult, error) {
	clientID, sessionID, err := validateOwnedSessionInput(input)
	if err != nil {
		return ListMessagesResult{}, err
	}
	if _, err := s.sessions.GetByID(ctx, model.OwnerKey(clientID), sessionID); err != nil {
		return ListMessagesResult{}, mapRepositoryError(err, "get session")
	}
	messages, err := s.messages.ListBySession(ctx, sessionID, 0)
	if err != nil {
		return ListMessagesResult{}, fmt.Errorf("list messages: %w", err)
	}
	return ListMessagesResult{Messages: messages}, nil
}

// SendMessage 持久化用户消息，构造受限上下文，获得助手回复并持久化结果。
func (s *Service) SendMessage(ctx context.Context, input SendMessageInput) (SendMessageResult, error) {
	clientID, sessionID, content, clientMessageID, profile, err := validateSendMessageInput(input)
	if err != nil {
		return SendMessageResult{}, err
	}
	ownerKey := model.OwnerKey(clientID)
	session, err := s.sessions.GetByID(ctx, ownerKey, sessionID)
	if err != nil {
		return SendMessageResult{}, mapRepositoryError(err, "get session")
	}
	if _, err := s.messages.GetByClientMessageID(ctx, sessionID, clientMessageID); err == nil {
		return SendMessageResult{}, ErrDuplicateRequest
	} else if !errors.Is(err, repository.ErrNotFound) {
		return SendMessageResult{}, fmt.Errorf("find client message: %w", err)
	}

	now := s.now()
	userMessage := model.Message{
		ID:              s.newID(),
		SessionID:       sessionID,
		Role:            model.RoleUser,
		Content:         content,
		Status:          model.StatusCompleted,
		ClientMessageID: &clientMessageID,
		CreatedAt:       now,
	}
	if userMessage.ID == "" {
		return SendMessageResult{}, fmt.Errorf("generate user message ID")
	}
	if _, err := s.messages.AppendMessage(ctx, &userMessage); err != nil {
		if errors.Is(err, repository.ErrDuplicateClientMessageID) {
			return SendMessageResult{}, ErrDuplicateRequest
		}
		return SendMessageResult{}, mapRepositoryError(err, "append user message")
	}

	persisted, err := s.messages.ListBySession(ctx, sessionID, 0)
	if err != nil {
		return SendMessageResult{}, fmt.Errorf("read message context: %w", err)
	}
	completion, err := s.completer.Complete(ctx, CompletionRequest{
		OwnerKey: ownerKey, SessionID: sessionID, ModelProfile: profile,
		Messages: boundedContext(persisted, userMessage.ID),
	})
	if err != nil {
		return SendMessageResult{}, fmt.Errorf("%w: %v", ErrAICompletion, err)
	}
	if strings.TrimSpace(completion.Content) == "" {
		return SendMessageResult{}, fmt.Errorf("%w: empty response", ErrAICompletion)
	}
	assistantMessage := model.Message{
		ID:               s.newID(),
		SessionID:        sessionID,
		Role:             model.RoleAssistant,
		Content:          completion.Content,
		Status:           model.StatusCompleted,
		ModelName:        optionalString(completion.ModelName),
		PromptTokens:     optionalInt(completion.PromptTokens),
		CompletionTokens: optionalInt(completion.CompletionTokens),
		CreatedAt:        s.now(),
	}
	if assistantMessage.ID == "" {
		return SendMessageResult{}, fmt.Errorf("generate assistant message ID")
	}
	if _, err := s.messages.AppendMessage(ctx, &assistantMessage); err != nil {
		return SendMessageResult{}, mapRepositoryError(err, "append assistant message")
	}

	title := session.Title
	if session.LastMessageAt == nil {
		title = firstMessageTitle(content)
	}
	if err := s.sessions.UpdateTitleAndTime(ctx, ownerKey, sessionID, title, assistantMessage.CreatedAt); err != nil {
		return SendMessageResult{}, mapRepositoryError(err, "update session")
	}
	return SendMessageResult{UserMessage: userMessage, AssistantMessage: assistantMessage}, nil
}

func validateSessionInput(clientID, modelProfile string) (string, string, error) {
	clientID, err := validateClientID(clientID)
	if err != nil {
		return "", "", err
	}
	profile := strings.TrimSpace(modelProfile)
	if profile == "" {
		profile = DefaultModelProfile
	}
	if utf8.RuneCountInString(profile) > maxModelProfile || profile != DefaultModelProfile {
		return "", "", fmt.Errorf("model_profile: %w", ErrInvalidArgument)
	}
	return clientID, profile, nil
}

func validateClientID(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !isUUID(value) {
		return "", fmt.Errorf("client_id: %w", ErrInvalidArgument)
	}
	return value, nil
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func validateOwnedSessionInput(input SessionInput) (string, string, error) {
	clientID, err := validateClientID(input.ClientID)
	if err != nil {
		return "", "", err
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" || utf8.RuneCountInString(sessionID) > 36 {
		return "", "", fmt.Errorf("session_id: %w", ErrInvalidArgument)
	}
	return clientID, sessionID, nil
}

func validateSendMessageInput(input SendMessageInput) (string, string, string, string, string, error) {
	clientID, sessionID, err := validateOwnedSessionInput(SessionInput{ClientID: input.ClientID, SessionID: input.SessionID})
	if err != nil {
		return "", "", "", "", "", err
	}
	content := strings.TrimSpace(input.Content)
	clientMessageID := strings.TrimSpace(input.ClientMessageID)
	if content == "" || utf8.RuneCountInString(content) > maxMessageRunes {
		return "", "", "", "", "", fmt.Errorf("content: %w", ErrInvalidArgument)
	}
	if clientMessageID == "" || utf8.RuneCountInString(clientMessageID) > maxClientMessageID {
		return "", "", "", "", "", fmt.Errorf("client_message_id: %w", ErrInvalidArgument)
	}
	_, profile, err := validateSessionInput(clientID, input.ModelProfile)
	if err != nil {
		return "", "", "", "", "", err
	}
	return clientID, sessionID, content, clientMessageID, profile, nil
}

func mapRepositoryError(err error, operation string) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrSessionNotFound
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func boundedContext(messages []model.Message, currentMessageID string) []CompletionMessage {
	context := make([]CompletionMessage, 0, len(messages))
	for _, message := range messages {
		if message.Status != model.StatusCompleted || strings.TrimSpace(message.Content) == "" {
			continue
		}
		context = append(context, CompletionMessage{ID: message.ID, Role: message.Role, Content: message.Content})
	}
	for contextCost(context) > contextTokenBudget && len(context) > 1 {
		if context[0].ID == currentMessageID {
			break
		}
		context = context[1:]
	}
	return context
}

func contextCost(messages []CompletionMessage) int {
	total := 0
	for _, message := range messages {
		// 每四个 Unicode 字符按一个 token 的保守近似计；至少计一个 token。
		cost := (utf8.RuneCountInString(message.Content) + 3) / 4
		if cost < 1 {
			cost = 1
		}
		total += cost
	}
	return total
}

func firstMessageTitle(content string) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) <= 32 {
		return string(runes)
	}
	return string(runes[:32]) + "…"
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}
