package chat

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"ai-flowmind/services/go-api/internal/model"
	"ai-flowmind/services/go-api/internal/repository"
)

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
		Sessions: repos, Messages: repos, Completer: &recordingCompleter{},
		Now: func() time.Time { return clock }, NewID: sequentialID(),
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
		Sessions: repos, Messages: repos, Completer: &recordingCompleter{},
		Now: time.Now, NewID: sequentialID(),
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
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Completer: completer, Now: time.Now, NewID: sequentialID()})
	created, _ := service.CreateSession(context.Background(), CreateSessionInput{ClientID: testClientID})
	input := SendMessageInput{ClientID: testClientID, SessionID: created.Session.ID, Content: "hi", ClientMessageID: "same-id"}
	if _, err := service.SendMessage(context.Background(), input); err != nil {
		t.Fatalf("first SendMessage() error = %v", err)
	}
	if _, err := service.SendMessage(context.Background(), input); !errors.Is(err, ErrDuplicateRequest) {
		t.Fatalf("duplicate SendMessage() error = %v, want ErrDuplicateRequest", err)
	}
	if completer.calls != 1 {
		t.Fatalf("completer calls = %d, want 1", completer.calls)
	}
}

func TestSendMessageMapsDatabaseDuplicateToDuplicateRequest(t *testing.T) {
	repos := newMemoryRepositories()
	completer := &recordingCompleter{}
	service, _ := New(Dependencies{Sessions: repos, Messages: repos, Completer: completer, Now: time.Now, NewID: sequentialID()})
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

func TestSendMessageReportsCompleterFailureAfterKeepingUserMessage(t *testing.T) {
	repos := newMemoryRepositories()
	service, _ := New(Dependencies{
		Sessions: repos, Messages: repos, Completer: &recordingCompleter{err: errors.New("unavailable")},
		Now: time.Now, NewID: sequentialID(),
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
	sessions  map[string]model.Session
	messages  map[string][]model.Message
	appendErr error
}

const testClientID = "123e4567-e89b-12d3-a456-426614174000"

func newMemoryRepositories() *memoryRepositories {
	return &memoryRepositories{sessions: map[string]model.Session{}, messages: map[string][]model.Message{}}
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

func sequentialID() func() string {
	n := 0
	return func() string {
		n++
		return "id-" + string(rune('0'+n))
	}
}
