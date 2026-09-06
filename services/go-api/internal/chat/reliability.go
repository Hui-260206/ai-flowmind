package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Reliability coordinates temporary Redis-backed send-message state. It never
// owns chat content or durable history; MySQL remains the source of truth.
type Reliability interface {
	Claim(context.Context, IdempotencyRequest) (IdempotencyClaim, error)
	// Reclaim gives a durable failed/interrupted operation a new token after its
	// session lock has been acquired. It is never used for an active workflow.
	Reclaim(context.Context, IdempotencyRequest) (IdempotencyClaim, error)
	AttachUserMessage(context.Context, IdempotencyClaim, string) error
	Complete(context.Context, IdempotencyClaim, string, string) error
	Fail(context.Context, IdempotencyClaim, string) error
	Abort(context.Context, IdempotencyClaim) error
	AcquireSession(context.Context, string) (SessionLock, bool, error)
	ReleaseSession(context.Context, SessionLock) error
	AllowOwner(context.Context, string, time.Time) (bool, error)
}

type IdempotencyRequest struct {
	OwnerKey        string
	SessionID       string
	ClientMessageID string
	Fingerprint     string
}

type ClaimState string

const (
	ClaimNew        ClaimState = "new"
	ClaimProcessing ClaimState = "processing"
	ClaimCompleted  ClaimState = "completed"
	ClaimFailed     ClaimState = "failed"
	ClaimMismatch   ClaimState = "mismatch"
)

// IdempotencyClaim contains no content. Token identifies the caller allowed to
// change the temporary Redis record.
type IdempotencyClaim struct {
	State              ClaimState
	Token              string
	OriginalState      ClaimState
	UserMessageID      string
	AssistantMessageID string
	Request            IdempotencyRequest
}

type SessionLock struct {
	SessionID string
	Token     string
}

func requestFingerprint(content, profile string) string {
	sum := sha256.Sum256([]byte(content + "\x00" + profile))
	return hex.EncodeToString(sum[:])
}

// MemoryReliability is a deterministic, concurrency-safe implementation for
// unit tests and local fake-mode HTTP tests. Production wiring uses Redis.
type MemoryReliability struct {
	mu          sync.Mutex
	claims      map[string]memoryClaim
	locks       map[string]string
	rateWindows map[string]int
	limit       int
}

type memoryClaim struct {
	state       ClaimState
	token       string
	request     IdempotencyRequest
	userID      string
	assistantID string
}

func NewMemoryReliability(limit int) *MemoryReliability {
	if limit < 1 {
		limit = 20
	}
	return &MemoryReliability{claims: map[string]memoryClaim{}, locks: map[string]string{}, rateWindows: map[string]int{}, limit: limit}
}

func (m *MemoryReliability) Claim(_ context.Context, request IdempotencyRequest) (IdempotencyClaim, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := memoryIdempotencyKey(request)
	if existing, ok := m.claims[key]; ok {
		if existing.request.Fingerprint != request.Fingerprint {
			return IdempotencyClaim{State: ClaimMismatch, Request: request}, nil
		}
		switch existing.state {
		case ClaimCompleted:
			return IdempotencyClaim{State: ClaimCompleted, Request: request, UserMessageID: existing.userID, AssistantMessageID: existing.assistantID}, nil
		case ClaimProcessing:
			return IdempotencyClaim{State: ClaimProcessing, Request: request, UserMessageID: existing.userID}, nil
		case ClaimFailed:
			token := memoryToken(key + ":retry")
			existing.state, existing.token = ClaimProcessing, token
			m.claims[key] = existing
			return IdempotencyClaim{State: ClaimFailed, OriginalState: ClaimFailed, Token: token, Request: request, UserMessageID: existing.userID}, nil
		}
	}
	token := memoryToken(key)
	m.claims[key] = memoryClaim{state: ClaimProcessing, token: token, request: request}
	return IdempotencyClaim{State: ClaimNew, OriginalState: ClaimNew, Token: token, Request: request}, nil
}

func (m *MemoryReliability) Reclaim(_ context.Context, request IdempotencyRequest) (IdempotencyClaim, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := memoryIdempotencyKey(request)
	record, ok := m.claims[key]
	if !ok || record.request.Fingerprint != request.Fingerprint || (record.state != ClaimProcessing && record.state != ClaimFailed) {
		return IdempotencyClaim{}, fmt.Errorf("memory reliability operation cannot be reclaimed")
	}
	token := memoryToken(key + ":reclaim:" + record.token)
	record.state, record.token = ClaimProcessing, token
	m.claims[key] = record
	return IdempotencyClaim{State: ClaimFailed, OriginalState: ClaimFailed, Token: token, Request: request, UserMessageID: record.userID, AssistantMessageID: record.assistantID}, nil
}

func (m *MemoryReliability) AttachUserMessage(_ context.Context, claim IdempotencyClaim, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.withProcessing(claim, func(record *memoryClaim) { record.userID = userID })
}
func (m *MemoryReliability) Complete(_ context.Context, claim IdempotencyClaim, userID, assistantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.withProcessing(claim, func(record *memoryClaim) {
		record.state, record.token, record.userID, record.assistantID = ClaimCompleted, "", userID, assistantID
	})
}
func (m *MemoryReliability) Fail(_ context.Context, claim IdempotencyClaim, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.withProcessing(claim, func(record *memoryClaim) { record.state, record.token, record.userID = ClaimFailed, "", userID })
}
func (m *MemoryReliability) Abort(_ context.Context, claim IdempotencyClaim) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := memoryIdempotencyKey(claim.Request)
	record, ok := m.claims[key]
	if !ok || record.state != ClaimProcessing || record.token != claim.Token {
		return nil
	}
	if claim.OriginalState == ClaimFailed {
		record.state, record.token, record.userID = ClaimFailed, "", claim.UserMessageID
		m.claims[key] = record
	} else {
		delete(m.claims, key)
	}
	return nil
}
func (m *MemoryReliability) AcquireSession(_ context.Context, sessionID string) (SessionLock, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.locks[sessionID]; exists {
		return SessionLock{}, false, nil
	}
	token := memoryToken(sessionID)
	m.locks[sessionID] = token
	return SessionLock{SessionID: sessionID, Token: token}, true, nil
}
func (m *MemoryReliability) ReleaseSession(_ context.Context, lock SessionLock) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locks[lock.SessionID] == lock.Token {
		delete(m.locks, lock.SessionID)
	}
	return nil
}
func (m *MemoryReliability) AllowOwner(_ context.Context, ownerKey string, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := ownerKey + ":" + now.UTC().Format("200601021504")
	m.rateWindows[key]++
	return m.rateWindows[key] <= m.limit, nil
}
func (m *MemoryReliability) withProcessing(claim IdempotencyClaim, mutate func(*memoryClaim)) error {
	key := memoryIdempotencyKey(claim.Request)
	record, ok := m.claims[key]
	if !ok || record.state != ClaimProcessing || record.token != claim.Token {
		return fmt.Errorf("memory reliability operation lost ownership")
	}
	mutate(&record)
	m.claims[key] = record
	return nil
}
func memoryIdempotencyKey(request IdempotencyRequest) string {
	return request.OwnerKey + "\x00" + request.SessionID + "\x00" + request.ClientMessageID
}
func memoryToken(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}
