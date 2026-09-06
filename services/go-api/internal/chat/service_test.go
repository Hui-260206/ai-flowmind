package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"ai-flowmind/services/go-api/internal/model"
)

func TestNewRejectsMissingDependency(t *testing.T) {
	base := testDependencies()
	tests := []struct {
		name   string
		mutate func(*Dependencies)
	}{
		{name: "sessions", mutate: func(deps *Dependencies) { deps.Sessions = nil }},
		{name: "messages", mutate: func(deps *Dependencies) { deps.Messages = nil }},
		{name: "completer", mutate: func(deps *Dependencies) { deps.Completer = nil }},
		{name: "clock", mutate: func(deps *Dependencies) { deps.Now = nil }},
		{name: "id generator", mutate: func(deps *Dependencies) { deps.NewID = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := base
			tt.mutate(&deps)
			service, err := New(deps)
			if service != nil {
				t.Fatal("New() service must be nil when a dependency is absent")
			}
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("New() error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

func TestNewAcceptsExplicitDependencies(t *testing.T) {
	service, err := New(testDependencies())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if service == nil || service.sessions == nil || service.messages == nil || service.completer == nil || service.now == nil || service.newID == nil {
		t.Fatalf("New() returned incomplete service: %#v", service)
	}
}

func testDependencies() Dependencies {
	return Dependencies{
		Sessions:  sessionRepositoryStub{},
		Messages:  messageRepositoryStub{},
		Completer: completerStub{},
		Now:       func() time.Time { return time.Unix(0, 0).UTC() },
		NewID:     func() string { return "test-id" },
	}
}

type sessionRepositoryStub struct{}

func (sessionRepositoryStub) Create(context.Context, *model.Session) error { return nil }
func (sessionRepositoryStub) GetByID(context.Context, string, string) (*model.Session, error) {
	return nil, nil
}
func (sessionRepositoryStub) ListByOwner(context.Context, string) ([]model.Session, error) {
	return nil, nil
}
func (sessionRepositoryStub) UpdateTitleAndTime(context.Context, string, string, string, time.Time) error {
	return nil
}
func (sessionRepositoryStub) SoftDelete(context.Context, string, string) error { return nil }

type messageRepositoryStub struct{}

func (messageRepositoryStub) AppendMessage(context.Context, *model.Message) (int64, error) {
	return 0, nil
}
func (messageRepositoryStub) ListBySession(context.Context, string, int) ([]model.Message, error) {
	return nil, nil
}
func (messageRepositoryStub) GetByClientMessageID(context.Context, string, string) (*model.Message, error) {
	return nil, nil
}

type completerStub struct{}

func (completerStub) Complete(context.Context, CompletionRequest) (CompletionResult, error) {
	return CompletionResult{}, nil
}
