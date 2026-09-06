package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"ai-flowmind/services/go-api/internal/chat"
	"ai-flowmind/services/go-api/internal/config"
	"ai-flowmind/services/go-api/internal/health"
	"ai-flowmind/services/go-api/internal/model"
	"ai-flowmind/services/go-api/internal/repository"
)

func TestChatHTTPWorkflowAndRequestID(t *testing.T) {
	server, repos := testChatServer(t)
	create := serve(server, http.MethodPost, "/api/v1/sessions", testHTTPClientIDA, "req-create", "")
	if create.Code != http.StatusCreated || create.Header().Get("X-Request-ID") != "req-create" {
		t.Fatalf("create response = %d %s", create.Code, create.Body.String())
	}
	var session sessionResponse
	decodeBody(t, create, &session)
	if session.Title != chat.InitialSessionTitle || session.ModelProfile != chat.DefaultModelProfile {
		t.Fatalf("created session = %#v", session)
	}
	ready := serve(server, http.MethodGet, "/readyz", "", "", "")
	var readiness health.Response
	decodeBody(t, ready, &readiness)
	if ready.Code != http.StatusOK || readiness.Status != "ready" || readiness.Checks["mysql"].Status != "ok" || readiness.Checks["redis"].Status != "ok" {
		t.Fatalf("Fake-mode readiness = %d %#v, want MySQL/Redis ready", ready.Code, readiness)
	}
	if _, found := readiness.Checks["python_grpc"]; found {
		t.Fatalf("Fake-mode readiness must not depend on python_grpc: %#v", readiness)
	}
	other := serve(server, http.MethodPost, "/api/v1/sessions", testHTTPClientIDB, "", "")
	if other.Code != http.StatusCreated {
		t.Fatalf("other create response = %d %s", other.Code, other.Body.String())
	}
	listed := serve(server, http.MethodGet, "/api/v1/sessions", testHTTPClientIDA, "", "")
	var sessions struct {
		Items []sessionResponse `json:"items"`
	}
	decodeBody(t, listed, &sessions)
	if listed.Code != http.StatusOK || len(sessions.Items) != 1 || sessions.Items[0].ID != session.ID {
		t.Fatalf("session list response = %d %#v", listed.Code, sessions)
	}

	send := serve(server, http.MethodPost, "/api/v1/sessions/"+session.ID+"/messages", testHTTPClientIDA, "req-send", `{"content":"你好","client_message_id":"m-1"}`)
	if send.Code != http.StatusOK {
		t.Fatalf("send response = %d %s", send.Code, send.Body.String())
	}
	var completed struct {
		RequestID        string          `json:"request_id"`
		UserMessage      messageResponse `json:"user_message"`
		AssistantMessage messageResponse `json:"assistant_message"`
	}
	decodeBody(t, send, &completed)
	if completed.RequestID != "req-send" || completed.UserMessage.Role != "user" || completed.AssistantMessage.Role != "assistant" {
		t.Fatalf("send body = %#v", completed)
	}

	history := serve(server, http.MethodGet, "/api/v1/sessions/"+session.ID+"/messages", testHTTPClientIDA, "", "")
	var messages struct {
		Items []messageResponse `json:"items"`
	}
	decodeBody(t, history, &messages)
	if history.Code != http.StatusOK || len(messages.Items) != 2 || messages.Items[0].Role != "user" || messages.Items[1].Role != "assistant" {
		t.Fatalf("history response = %d %#v", history.Code, messages)
	}

	duplicate := serve(server, http.MethodPost, "/api/v1/sessions/"+session.ID+"/messages", testHTTPClientIDA, "", `{"content":"你好","client_message_id":"m-1"}`)
	assertAPIError(t, duplicate, http.StatusConflict, "DUPLICATE_REQUEST")
	if len(repos.messages[session.ID]) != 2 {
		t.Fatalf("duplicate request wrote messages: %#v", repos.messages[session.ID])
	}

	deleted := serve(server, http.MethodDelete, "/api/v1/sessions/"+session.ID, testHTTPClientIDA, "", "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete response = %d %s", deleted.Code, deleted.Body.String())
	}
	assertAPIError(t, serve(server, http.MethodGet, "/api/v1/sessions/"+session.ID+"/messages", testHTTPClientIDA, "", ""), http.StatusNotFound, "SESSION_NOT_FOUND")
}

func TestChatHTTPValidatesInputAndConcealsOtherOwner(t *testing.T) {
	server, _ := testChatServer(t)
	assertAPIError(t, serve(server, http.MethodPost, "/api/v1/sessions", "", "client-error", ""), http.StatusBadRequest, "INVALID_ARGUMENT")
	created := serve(server, http.MethodPost, "/api/v1/sessions", testHTTPClientIDA, "", "")
	var session sessionResponse
	decodeBody(t, created, &session)
	assertAPIError(t, serve(server, http.MethodPost, "/api/v1/sessions/"+session.ID+"/messages", testHTTPClientIDB, "", `{"content":"hello","client_message_id":"m-1"}`), http.StatusNotFound, "SESSION_NOT_FOUND")
	assertAPIError(t, serve(server, http.MethodPost, "/api/v1/sessions/"+session.ID+"/messages", testHTTPClientIDA, "", `{"content":" ","client_message_id":"m-1"}`), http.StatusBadRequest, "INVALID_ARGUMENT")
	assertAPIError(t, serve(server, http.MethodPost, "/api/v1/sessions/"+session.ID+"/messages", testHTTPClientIDA, "", `{`), http.StatusBadRequest, "INVALID_ARGUMENT")
}

const (
	testHTTPClientIDA = "123e4567-e89b-12d3-a456-426614174000"
	testHTTPClientIDB = "123e4567-e89b-12d3-a456-426614174001"
)

func testChatServer(t *testing.T) (*Server, *httpMemoryRepository) {
	t.Helper()
	repos := &httpMemoryRepository{sessions: map[string]model.Session{}, messages: map[string][]model.Message{}}
	id := 0
	service, err := chat.New(chat.Dependencies{
		Sessions: repos, Messages: repos, Completer: chat.FakeCompleter{}, Now: time.Now,
		NewID: func() string { id++; return "session-or-message-" + string(rune('0'+id)) },
	})
	if err != nil {
		t.Fatalf("chat.New() error = %v", err)
	}
	return New(config.HTTPConfig{Addr: ":8080"}, nil, Dependencies{
		Chat:       service,
		MySQL:      func(context.Context) error { return nil },
		Redis:      func(context.Context) error { return nil },
		PythonGRPC: func(context.Context) error { return errors.New("unavailable") },
	}), repos
}

func serve(server *Server, method, path, clientID, requestID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if clientID != "" {
		req.Header.Set(clientIDHeader, clientID)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	return res
}

func decodeBody(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode body %q: %v", response.Body.String(), err)
	}
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body struct {
		RequestID string `json:"request_id"`
		Error     struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeBody(t, response, &body)
	if response.Code != status || body.RequestID == "" || body.Error.Code != code {
		t.Fatalf("error response = %d %#v, want %d/%s", response.Code, body, status, code)
	}
}

type httpMemoryRepository struct {
	sessions map[string]model.Session
	messages map[string][]model.Message
}

func (r *httpMemoryRepository) Create(_ context.Context, session *model.Session) error {
	r.sessions[session.ID] = *session
	return nil
}
func (r *httpMemoryRepository) GetByID(_ context.Context, ownerKey, id string) (*model.Session, error) {
	session, ok := r.sessions[id]
	if !ok || session.OwnerKey != ownerKey || session.DeletedAt != nil {
		return nil, repository.ErrNotFound
	}
	return &session, nil
}
func (r *httpMemoryRepository) ListByOwner(_ context.Context, ownerKey string) ([]model.Session, error) {
	values := []model.Session{}
	for _, session := range r.sessions {
		if session.OwnerKey == ownerKey && session.DeletedAt == nil {
			values = append(values, session)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	return values, nil
}
func (r *httpMemoryRepository) UpdateTitleAndTime(_ context.Context, ownerKey, id, title string, last time.Time) error {
	session, err := r.GetByID(context.Background(), ownerKey, id)
	if err != nil {
		return err
	}
	session.Title, session.LastMessageAt, session.UpdatedAt = title, &last, last
	r.sessions[id] = *session
	return nil
}
func (r *httpMemoryRepository) SoftDelete(_ context.Context, ownerKey, id string) error {
	session, err := r.GetByID(context.Background(), ownerKey, id)
	if err != nil {
		return err
	}
	now := time.Now()
	session.DeletedAt = &now
	r.sessions[id] = *session
	return nil
}
func (r *httpMemoryRepository) AppendMessage(_ context.Context, message *model.Message) (int64, error) {
	if _, ok := r.sessions[message.SessionID]; !ok {
		return 0, repository.ErrNotFound
	}
	for _, old := range r.messages[message.SessionID] {
		if message.ClientMessageID != nil && old.ClientMessageID != nil && *message.ClientMessageID == *old.ClientMessageID {
			return 0, errors.New("duplicate")
		}
	}
	message.Seq = int64(len(r.messages[message.SessionID]) + 1)
	r.messages[message.SessionID] = append(r.messages[message.SessionID], *message)
	return message.Seq, nil
}
func (r *httpMemoryRepository) ListBySession(_ context.Context, id string, limit int) ([]model.Message, error) {
	return append([]model.Message(nil), r.messages[id]...), nil
}
func (r *httpMemoryRepository) GetByClientMessageID(_ context.Context, id, clientID string) (*model.Message, error) {
	for _, message := range r.messages[id] {
		if message.ClientMessageID != nil && *message.ClientMessageID == clientID {
			return &message, nil
		}
	}
	return nil, repository.ErrNotFound
}
