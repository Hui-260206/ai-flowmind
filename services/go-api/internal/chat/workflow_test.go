package chat

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"ai-flowmind/services/go-api/internal/metrics"
	"ai-flowmind/services/go-api/internal/model"
	"ai-flowmind/services/go-api/internal/repository"
)

type recordingMetrics struct{ persisted []string }

func (m *recordingMetrics) ObserveChatRequest(string, string, int, time.Duration) {}
func (m *recordingMetrics) ObserveGRPC(string, time.Duration)                     {}
func (m *recordingMetrics) IncAITimeout()                                         {}
func (m *recordingMetrics) IncMessagePersistFailure(stage string) {
	m.persisted = append(m.persisted, stage)
}
func (m *recordingMetrics) IncRedisLockFailure(string) {}

var _ metrics.ChatMetrics = (*recordingMetrics)(nil)

func TestFakeCompleterReturnsDeterministicCompletedResult(t *testing.T) {
	result, err := (FakeCompleter{}).Complete(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.Content != fakeReply || result.ModelName != "go-fake" {
		t.Fatalf("unexpected fake result: %#v", result)
	}
}

func TestSendMessagePersistsPairUpdatesFirstTitleAndPreservesLaterTitle(t *testing.T) {
	repos := newMemoryRepositories()
	clock := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: &recordingCompleter{},
		Reliability: NewMemoryReliability(20),
		Now:         func() time.Time { return clock }, NewID: sequentialID(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	created, err := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.Session.Title != InitialSessionTitle {
		t.Fatalf("initial title = %q, want %q", created.Session.Title, InitialSessionTitle)
	}

	first, err := service.SendMessage(context.Background(), SendMessageInput{
		ClientID: testClientID, SessionID: created.Session.ID, Content: "  第一条消息  ", ClientMessageID: "msg-1",
	})
	if err != nil {
		t.Fatalf("first SendMessage() error = %v", err)
	}
	if first.UserMessage.Seq != 1 || first.AssistantMessage.Seq != 2 || first.AssistantMessage.Content != "助手回复" {
		t.Fatalf("unexpected first result: %#v", first)
	}
	afterFirst, err := repos.GetByID(context.Background(), model.OwnerKey(testClientID), created.Session.ID)
	if err != nil {
		t.Fatalf("get after first message: %v", err)
	}
	if afterFirst.Title != "第一条消息" || afterFirst.LastMessageAt == nil {
		t.Fatalf("unexpected session after first message: %#v", afterFirst)
	}

	clock = clock.Add(time.Minute)
	if _, err := service.SendMessage(context.Background(), SendMessageInput{
		ClientID: testClientID, SessionID: created.Session.ID, Content: "第二条消息", ClientMessageID: "msg-2",
	}); err != nil {
		t.Fatalf("second SendMessage() error = %v", err)
	}
	afterSecond, _ := repos.GetByID(context.Background(), model.OwnerKey(testClientID), created.Session.ID)
	if afterSecond.Title != "第一条消息" {
		t.Fatalf("later message changed title to %q", afterSecond.Title)
	}
}

func TestSendMessageRejectsInvalidInputWithoutPersistence(t *testing.T) {
	repos := newMemoryRepositories()
	service, err := New(Dependencies{
		Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: &recordingCompleter{},
		Reliability: NewMemoryReliability(20),
		Now:         time.Now, NewID: sequentialID(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	created, err := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	tests := []SendMessageInput{
		{ClientID: testClientID, SessionID: created.Session.ID, Content: " ", ClientMessageID: "x"},
		{ClientID: testClientID, SessionID: created.Session.ID, Content: "x", ClientMessageID: " "},
		{ClientID: testClientID, SessionID: created.Session.ID, Content: strings.Repeat("你", maxMessageRunes+1), ClientMessageID: "x"},
		{ClientID: testClientID, SessionID: created.Session.ID, Content: "x", ClientMessageID: strings.Repeat("x", maxClientMessageID+1)},
	}
	for _, input := range tests {
		if _, err := service.SendMessage(context.Background(), input); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("SendMessage(%#v) error = %v, want ErrInvalidArgument", input, err)
		}
	}
	messages, err := repos.ListBySession(context.Background(), created.Session.ID, 0)
	if err != nil || len(messages) != 0 {
		t.Fatalf("invalid requests persisted messages: %#v, err=%v", messages, err)
	}
}

func TestSendMessageRejectsDuplicateWithoutSecondCompletion(t *testing.T) {
	repos := newMemoryRepositories()
	completer := &recordingCompleter{}
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: completer, Reliability: NewMemoryReliability(20), Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	input := SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "hi", ClientMessageID: "same-id"}
	if _, err := service.SendMessage(context.Background(), input); err != nil {
		t.Fatalf("first SendMessage() error = %v", err)
	}
	second, err := service.SendMessage(context.Background(), input)
	if err != nil {
		t.Fatalf("duplicate SendMessage() error = %v", err)
	}
	if second.UserMessage.ID != firstUserMessageID(repos, created.Session.ID) || second.AssistantMessage.Content != "助手回复" {
		t.Fatalf("duplicate result = %#v, want replayed message pair", second)
	}
	if completer.calls != 1 {
		t.Fatalf("completer calls = %d, want 1", completer.calls)
	}
}

func firstUserMessageID(repos *memoryRepositories, sessionID string) string {
	return repos.messages[sessionID][0].ID
}

func TestSendMessageMapsDatabaseDuplicateToDuplicateRequest(t *testing.T) {
	repos := newMemoryRepositories()
	completer := &recordingCompleter{}
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: completer, Reliability: NewMemoryReliability(20), Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	repos.appendErr = repository.ErrDuplicateClientMessageID

	_, err := service.SendMessage(context.Background(), SendMessageInput{
		ClientID: testClientID, SessionID: created.Session.ID, Content: "hi", ClientMessageID: "same-id",
	})
	if !errors.Is(err, ErrDuplicateRequest) {
		t.Fatalf("SendMessage() error = %v, want ErrDuplicateRequest", err)
	}
	if completer.calls != 0 {
		t.Fatalf("completer calls = %d, want 0", completer.calls)
	}
}

func TestSendMessageRecordsPersistenceFailure(t *testing.T) {
	repos := newMemoryRepositories()
	repos.appendErr = errors.New("database unavailable")
	collector := &recordingMetrics{}
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: &recordingCompleter{}, Reliability: NewMemoryReliability(20), Now: time.Now, NewID: sequentialID(), Metrics: collector})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	_, err := service.SendMessage(context.Background(), SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "persist", ClientMessageID: "persist-failure"})
	if err == nil || len(collector.persisted) != 1 || collector.persisted[0] != "user" {
		t.Fatalf("SendMessage() / persistence metrics = %v / %#v", err, collector.persisted)
	}
}

func TestSendMessageReportsCompleterFailureAfterKeepingUserMessage(t *testing.T) {
	repos := newMemoryRepositories()
	reliability := NewMemoryReliability(20)
	completer := &recordingCompleter{err: errors.New("unavailable")}
	service, _ := New(Dependencies{
		Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: completer,
		Reliability: reliability,
		Now:         time.Now, NewID: sequentialID(),
	})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	_, err := service.SendMessage(context.Background(), SendMessageInput{
		ClientID: testClientID, SessionID: created.Session.ID, Content: "hello", ClientMessageID: "m-1",
	})
	if !errors.Is(err, ErrAICompletion) {
		t.Fatalf("SendMessage() error = %v, want ErrAICompletion", err)
	}
	messages, _ := repos.ListBySession(context.Background(), created.Session.ID, 0)
	if len(messages) != 1 || messages[0].Role != model.RoleUser {
		t.Fatalf("AI failure should retain only user message, got %#v", messages)
	}
	completer.err = nil
	result, err := service.SendMessage(context.Background(), SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "hello", ClientMessageID: "m-1"})
	if err != nil || result.UserMessage.ID != messages[0].ID || result.AssistantMessage.Role != model.RoleAssistant {
		t.Fatalf("failed request resume = %#v, %v", result, err)
	}
	messages, _ = repos.ListBySession(context.Background(), created.Session.ID, 0)
	if len(messages) != 2 || completer.calls != 2 {
		t.Fatalf("resume messages/calls = %d/%d, want 2/2", len(messages), completer.calls)
	}
}

func TestSendMessageRejectsBusySessionWithoutPersistence(t *testing.T) {
	repos := newMemoryRepositories()
	reliability := NewMemoryReliability(20)
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: &recordingCompleter{}, Reliability: reliability, Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	lock, ok, err := reliability.AcquireSession(context.Background(), created.Session.ID)
	if err != nil || !ok {
		t.Fatalf("AcquireSession() = %#v, %v", lock, err)
	}
	_, err = service.SendMessage(context.Background(), SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "blocked", ClientMessageID: "busy"})
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("SendMessage() error = %v, want ErrSessionBusy", err)
	}
	messages, _ := repos.ListBySession(context.Background(), created.Session.ID, 0)
	if len(messages) != 0 {
		t.Fatalf("busy request persisted messages: %#v", messages)
	}
	_ = reliability.ReleaseSession(context.Background(), lock)
}

func TestSendMessageRateLimitAndCompletedReplay(t *testing.T) {
	repos := newMemoryRepositories()
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: &recordingCompleter{}, Reliability: NewMemoryReliability(1), Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	input := SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "first", ClientMessageID: "first"}
	if _, err := service.SendMessage(context.Background(), input); err != nil {
		t.Fatalf("first SendMessage() error = %v", err)
	}
	if _, err := service.SendMessage(context.Background(), input); err != nil {
		t.Fatalf("completed replay error = %v", err)
	}
	_, err := service.SendMessage(context.Background(), SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "second", ClientMessageID: "second"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second SendMessage() error = %v, want ErrRateLimited", err)
	}
}

func TestBusySessionDoesNotConsumeOwnerRateLimit(t *testing.T) {
	repos := newMemoryRepositories()
	reliability := NewMemoryReliability(1)
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: &recordingCompleter{}, Reliability: reliability, Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	lock, ok, err := reliability.AcquireSession(context.Background(), created.Session.ID)
	if err != nil || !ok {
		t.Fatalf("AcquireSession() = %#v, %v", lock, err)
	}
	_, err = service.SendMessage(context.Background(), SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "busy", ClientMessageID: "busy"})
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("busy SendMessage() error = %v", err)
	}
	_ = reliability.ReleaseSession(context.Background(), lock)
	if _, err := service.SendMessage(context.Background(), SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "accepted", ClientMessageID: "accepted"}); err != nil {
		t.Fatalf("request after busy error = %v; busy request must not consume quota", err)
	}
}

func TestSendMessageFallsBackToMySQLDuplicateWhenIdempotencyRecordIsLost(t *testing.T) {
	repos := newMemoryRepositories()
	firstReliability := NewMemoryReliability(20)
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: &recordingCompleter{}, Reliability: firstReliability, Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	input := SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "first", ClientMessageID: "same"}
	if _, err := service.SendMessage(context.Background(), input); err != nil {
		t.Fatalf("first SendMessage() error = %v", err)
	}
	service.reliability = NewMemoryReliability(20) // Simulates expired/lost Redis volatile state.
	result, err := service.SendMessage(context.Background(), input)
	if err != nil || result.AssistantMessage.Role != model.RoleAssistant {
		t.Fatalf("lost-record retry result = %#v, %v; want durable replay", result, err)
	}
}

type flakyReliability struct {
	*MemoryReliability
	failAttach   bool
	failComplete bool
}

func (f *flakyReliability) AttachUserMessage(ctx context.Context, claim IdempotencyClaim, userID string) error {
	if f.failAttach {
		f.failAttach = false
		return errors.New("redis response lost after user message")
	}
	return f.MemoryReliability.AttachUserMessage(ctx, claim, userID)
}

func (f *flakyReliability) Complete(ctx context.Context, claim IdempotencyClaim, userID, assistantID string) error {
	if f.failComplete {
		f.failComplete = false
		return errors.New("redis response lost after completion")
	}
	return f.MemoryReliability.Complete(ctx, claim, userID, assistantID)
}

func TestSendMessageRecoversAfterRedisAttachFailure(t *testing.T) {
	repos := newMemoryRepositories()
	reliability := &flakyReliability{MemoryReliability: NewMemoryReliability(20), failAttach: true}
	completer := &recordingCompleter{}
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: completer, Reliability: reliability, Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	input := SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "recover", ClientMessageID: "recover-attach"}
	if _, err := service.SendMessage(context.Background(), input); !errors.Is(err, ErrRedisUnavailable) {
		t.Fatalf("first SendMessage() error = %v, want ErrRedisUnavailable", err)
	}
	result, err := service.SendMessage(context.Background(), input)
	if err != nil || result.UserMessage.Seq != 1 || result.AssistantMessage.Seq != 2 || completer.calls != 1 {
		t.Fatalf("recovered SendMessage() = %#v, %v; calls=%d", result, err, completer.calls)
	}
}

func TestSendMessageReplaysAfterRedisCompletionFailure(t *testing.T) {
	repos := newMemoryRepositories()
	reliability := &flakyReliability{MemoryReliability: NewMemoryReliability(20), failComplete: true}
	completer := &recordingCompleter{}
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: completer, Reliability: reliability, Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	input := SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "recover", ClientMessageID: "recover-complete"}
	if _, err := service.SendMessage(context.Background(), input); !errors.Is(err, ErrRedisUnavailable) {
		t.Fatalf("first SendMessage() error = %v, want ErrRedisUnavailable", err)
	}
	result, err := service.SendMessage(context.Background(), input)
	if err != nil || result.AssistantMessage.Seq != 2 || completer.calls != 1 {
		t.Fatalf("replayed SendMessage() = %#v, %v; calls=%d", result, err, completer.calls)
	}
}

func TestSendMessageRecoversAssistantAfterSessionMetadataFailure(t *testing.T) {
	repos := newMemoryRepositories()
	completer := &recordingCompleter{}
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: completer, Reliability: NewMemoryReliability(20), Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	input := SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "recover", ClientMessageID: "recover-metadata"}
	repos.updateErr = errors.New("transient session update failure")
	if _, err := service.SendMessage(context.Background(), input); err == nil {
		t.Fatal("first SendMessage() error = nil, want metadata error")
	}
	repos.updateErr = nil
	result, err := service.SendMessage(context.Background(), input)
	if err != nil || result.AssistantMessage.Seq != 2 || completer.calls != 1 {
		t.Fatalf("recovered SendMessage() = %#v, %v; calls=%d", result, err, completer.calls)
	}
}

type failingReliability struct{ *MemoryReliability }

func (f failingReliability) Claim(context.Context, IdempotencyRequest) (IdempotencyClaim, error) {
	return IdempotencyClaim{}, errors.New("redis down")
}

func TestSendMessageFailsClosedWhenReliabilityUnavailable(t *testing.T) {
	repos := newMemoryRepositories()
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Operations: memorySendOperations{repos}, Completer: &recordingCompleter{}, Reliability: failingReliability{NewMemoryReliability(20)}, Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	_, err := service.SendMessage(context.Background(), SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "first", ClientMessageID: "first"})
	if !errors.Is(err, ErrRedisUnavailable) {
		t.Fatalf("SendMessage() error = %v, want ErrRedisUnavailable", err)
	}
	messages, _ := repos.ListBySession(context.Background(), created.Session.ID, 0)
	if len(messages) != 0 {
		t.Fatalf("Redis-unavailable request persisted messages: %#v", messages)
	}
}

func TestClientIDMustBeUUID(t *testing.T) {
	for _, value := range []string{"", "client-a", strings.Repeat("a", 128), "123e4567-e89b-12d3-a456-42661417400z"} {
		if _, err := validateClientID(value); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("validateClientID(%q) error = %v, want ErrInvalidArgument", value, err)
		}
	}
	clientID, err := validateClientID("123E4567-E89B-12D3-A456-426614174000")
	if err != nil || clientID != testClientID {
		t.Fatalf("validateClientID() = %q, %v", clientID, err)
	}
}

func TestBoundedContextDropsOldestAndRetainsCurrentMessage(t *testing.T) {
	old := model.Message{ID: "old", Role: model.RoleUser, Status: model.StatusCompleted, Content: strings.Repeat("a", contextTokenBudget*4)}
	current := model.Message{ID: "current", Role: model.RoleUser, Status: model.StatusCompleted, Content: "current"}
	context := boundedContext([]model.Message{old, current}, current.ID)
	if len(context) != 1 || context[0].ID != current.ID {
		t.Fatalf("context = %#v, want only current message", context)
	}
}

func TestFirstMessageTitleTruncatesRunes(t *testing.T) {
	title := firstMessageTitle(strings.Repeat("你", 33))
	if []rune(title)[32] != '…' || len([]rune(title)) != 33 {
		t.Fatalf("title = %q, want 32 runes plus ellipsis", title)
	}
}

type recordingCompleter struct {
	calls    int
	requests []CompletionRequest
	err      error
}

func (c *recordingCompleter) Complete(_ context.Context, request CompletionRequest) (CompletionResult, error) {
	c.calls++
	c.requests = append(c.requests, request)
	if c.err != nil {
		return CompletionResult{}, c.err
	}
	return CompletionResult{Content: "助手回复", ModelName: "test-fake"}, nil
}

type memoryRepositories struct {
	sessions   map[string]model.Session
	messages   map[string][]model.Message
	operations map[string]model.SendOperation
	appendErr  error
	updateErr  error
}

const testClientID = "123e4567-e89b-12d3-a456-426614174000"

func newMemoryRepositories() *memoryRepositories {
	return &memoryRepositories{sessions: map[string]model.Session{}, messages: map[string][]model.Message{}, operations: map[string]model.SendOperation{}}
}

func (r *memoryRepositories) Create(_ context.Context, session *model.Session) error {
	r.sessions[session.ID] = *session
	return nil
}

func (r *memoryRepositories) GetByID(_ context.Context, ownerKey, id string) (*model.Session, error) {
	session, ok := r.sessions[id]
	if !ok || session.OwnerKey != ownerKey || session.DeletedAt != nil {
		return nil, repository.ErrNotFound
	}
	return &session, nil
}

func (r *memoryRepositories) ListByOwner(_ context.Context, ownerKey string) ([]model.Session, error) {
	var sessions []model.Session
	for _, session := range r.sessions {
		if session.OwnerKey == ownerKey && session.DeletedAt == nil {
			sessions = append(sessions, session)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt) })
	return sessions, nil
}

func (r *memoryRepositories) UpdateTitleAndTime(_ context.Context, ownerKey, id, title string, lastMessageAt time.Time) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	session, ok := r.sessions[id]
	if !ok || session.OwnerKey != ownerKey || session.DeletedAt != nil {
		return repository.ErrNotFound
	}
	session.Title, session.LastMessageAt, session.UpdatedAt = title, &lastMessageAt, lastMessageAt
	r.sessions[id] = session
	return nil
}

func (r *memoryRepositories) SoftDelete(_ context.Context, ownerKey, id string) error {
	session, ok := r.sessions[id]
	if !ok || session.OwnerKey != ownerKey || session.DeletedAt != nil {
		return repository.ErrNotFound
	}
	now := time.Now()
	session.DeletedAt = &now
	r.sessions[id] = session
	return nil
}

func (r *memoryRepositories) AppendMessage(_ context.Context, message *model.Message) (int64, error) {
	if r.appendErr != nil {
		return 0, r.appendErr
	}
	if _, ok := r.sessions[message.SessionID]; !ok {
		return 0, repository.ErrNotFound
	}
	for _, existing := range r.messages[message.SessionID] {
		if message.ClientMessageID != nil && existing.ClientMessageID != nil && *message.ClientMessageID == *existing.ClientMessageID {
			return 0, errors.New("duplicate client message ID")
		}
	}
	message.Seq = int64(len(r.messages[message.SessionID]) + 1)
	r.messages[message.SessionID] = append(r.messages[message.SessionID], *message)
	return message.Seq, nil
}

func (r *memoryRepositories) ListBySession(_ context.Context, sessionID string, limit int) ([]model.Message, error) {
	values := append([]model.Message(nil), r.messages[sessionID]...)
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

func (r *memoryRepositories) GetByClientMessageID(_ context.Context, sessionID, clientMessageID string) (*model.Message, error) {
	for _, message := range r.messages[sessionID] {
		if message.ClientMessageID != nil && *message.ClientMessageID == clientMessageID {
			return &message, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (r *memoryRepositories) GetMessageByID(_ context.Context, sessionID, id string) (*model.Message, error) {
	for _, message := range r.messages[sessionID] {
		if message.ID == id {
			return &message, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (r *memoryRepositories) GetAssistantByOperation(_ context.Context, sessionID, operationID string) (*model.Message, error) {
	for _, message := range r.messages[sessionID] {
		if message.Role == model.RoleAssistant && message.SendOperationID != nil && *message.SendOperationID == operationID {
			return &message, nil
		}
	}
	return nil, repository.ErrNotFound
}

type memorySendOperations struct{ repositories *memoryRepositories }

func (r memorySendOperations) Get(_ context.Context, ownerKey, sessionID, clientMessageID string) (*model.SendOperation, error) {
	operation, ok := r.repositories.operations[operationKey(ownerKey, sessionID, clientMessageID)]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return &operation, nil
}
func (r memorySendOperations) Create(_ context.Context, operation *model.SendOperation) error {
	key := operationKey(operation.OwnerKey, operation.SessionID, operation.ClientMessageID)
	if _, exists := r.repositories.operations[key]; exists {
		return repository.ErrDuplicateClientMessageID
	}
	r.repositories.operations[key] = *operation
	return nil
}
func (r memorySendOperations) SetUserMessage(_ context.Context, ownerKey, sessionID, clientMessageID, messageID string) error {
	return r.repositories.updateOperation(ownerKey, sessionID, clientMessageID, func(operation *model.SendOperation) { operation.UserMessageID = messageID })
}
func (r memorySendOperations) SetAssistantMessage(_ context.Context, ownerKey, sessionID, clientMessageID, messageID string) error {
	return r.repositories.updateOperation(ownerKey, sessionID, clientMessageID, func(operation *model.SendOperation) { operation.AssistantMessageID = messageID })
}
func (r memorySendOperations) MarkFailed(_ context.Context, ownerKey, sessionID, clientMessageID string) error {
	return r.repositories.updateOperation(ownerKey, sessionID, clientMessageID, func(operation *model.SendOperation) { operation.Status = repository.SendOperationFailed })
}
func (r memorySendOperations) MarkCompleted(_ context.Context, ownerKey, sessionID, clientMessageID string) error {
	return r.repositories.updateOperation(ownerKey, sessionID, clientMessageID, func(operation *model.SendOperation) { operation.Status = repository.SendOperationCompleted })
}
func (r *memoryRepositories) updateOperation(ownerKey, sessionID, clientMessageID string, update func(*model.SendOperation)) error {
	key := operationKey(ownerKey, sessionID, clientMessageID)
	operation, ok := r.operations[key]
	if !ok {
		return repository.ErrNotFound
	}
	update(&operation)
	r.operations[key] = operation
	return nil
}
func operationKey(ownerKey, sessionID, clientMessageID string) string {
	return ownerKey + "\x00" + sessionID + "\x00" + clientMessageID
}

func sequentialID() func() string {
	n := 0
	return func() string {
		n++
		return "id-" + string(rune('0'+n))
	}
}
