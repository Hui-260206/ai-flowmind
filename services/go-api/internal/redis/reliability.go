package redis

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"ai-flowmind/services/go-api/internal/chat"
)

const (
	claimScript = `
local raw = redis.call('GET', KEYS[1])
if not raw then
  local record = { status = 'processing', token = ARGV[2], fingerprint = ARGV[1] }
  redis.call('PSETEX', KEYS[1], ARGV[3], cjson.encode(record))
  return { 'new', ARGV[2], '', '' }
end
local record = cjson.decode(raw)
if record.fingerprint ~= ARGV[1] then return { 'mismatch', '', '', '' } end
if record.status == 'completed' then return { 'completed', '', record.user_message_id or '', record.assistant_message_id or '' } end
if record.status == 'processing' then return { 'processing', '', record.user_message_id or '', '' } end
if record.status == 'failed' then
  record.status = 'processing'
  record.token = ARGV[2]
  redis.call('PSETEX', KEYS[1], ARGV[3], cjson.encode(record))
  return { 'failed', ARGV[2], record.user_message_id or '', '' }
end
return { 'mismatch', '', '', '' }`
	reclaimScript = `
local raw = redis.call('GET', KEYS[1]); if not raw then return { 'missing', '', '', '' } end
local record = cjson.decode(raw)
if record.fingerprint ~= ARGV[1] then return { 'mismatch', '', '', '' } end
if record.status ~= 'processing' and record.status ~= 'failed' then return { record.status, '', record.user_message_id or '', record.assistant_message_id or '' } end
record.status = 'processing'; record.token = ARGV[2]
redis.call('PSETEX', KEYS[1], ARGV[3], cjson.encode(record))
return { 'failed', ARGV[2], record.user_message_id or '', record.assistant_message_id or '' }`
	attachUserScript = `
local raw = redis.call('GET', KEYS[1]); if not raw then return 0 end
local record = cjson.decode(raw)
if record.status ~= 'processing' or record.token ~= ARGV[1] then return 0 end
record.user_message_id = ARGV[2]
redis.call('PSETEX', KEYS[1], ARGV[3], cjson.encode(record)); return 1`
	completeScript = `
local raw = redis.call('GET', KEYS[1]); if not raw then return 0 end
local record = cjson.decode(raw)
if record.status ~= 'processing' or record.token ~= ARGV[1] then return 0 end
record.status = 'completed'; record.user_message_id = ARGV[2]; record.assistant_message_id = ARGV[3]; record.token = nil
redis.call('PSETEX', KEYS[1], ARGV[4], cjson.encode(record)); return 1`
	failScript = `
local raw = redis.call('GET', KEYS[1]); if not raw then return 0 end
local record = cjson.decode(raw)
if record.status ~= 'processing' or record.token ~= ARGV[1] then return 0 end
record.status = 'failed'; record.user_message_id = ARGV[2]; record.token = nil
redis.call('PSETEX', KEYS[1], ARGV[3], cjson.encode(record)); return 1`
	abortScript = `
local raw = redis.call('GET', KEYS[1]); if not raw then return 0 end
local record = cjson.decode(raw)
if record.status ~= 'processing' or record.token ~= ARGV[1] then return 0 end
if ARGV[2] == 'failed' then
  record.status = 'failed'; record.token = nil; record.user_message_id = ARGV[3]
  redis.call('PSETEX', KEYS[1], ARGV[4], cjson.encode(record))
else redis.call('DEL', KEYS[1]) end
return 1`
	releaseLockScript = `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end return 0`
	rateLimitScript   = `local count = redis.call('INCR', KEYS[1]); if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end; if count > tonumber(ARGV[2]) then return 0 end; return 1`
)

// ReliabilityAdapter is the Redis implementation of chat.Reliability.
type ReliabilityAdapter struct {
	client         *Client
	idempotencyTTL time.Duration
	lockTTL        time.Duration
	ownerLimit     int
}

func NewReliabilityAdapter(client *Client, idempotencyTTL, lockTTL time.Duration, ownerLimit int) (*ReliabilityAdapter, error) {
	if client == nil || client.Client() == nil || idempotencyTTL <= 0 || lockTTL <= 0 || ownerLimit < 1 {
		return nil, fmt.Errorf("invalid Redis reliability configuration")
	}
	return &ReliabilityAdapter{client: client, idempotencyTTL: idempotencyTTL, lockTTL: lockTTL, ownerLimit: ownerLimit}, nil
}

func (a *ReliabilityAdapter) Claim(ctx context.Context, request chat.IdempotencyRequest) (chat.IdempotencyClaim, error) {
	token, err := randomToken()
	if err != nil {
		return chat.IdempotencyClaim{}, err
	}
	values, err := scriptStrings(ctx, a.client.Client().Eval(ctx, claimScript, []string{idempotencyKey(request)}, request.Fingerprint, token, milliseconds(a.idempotencyTTL)))
	if err != nil {
		return chat.IdempotencyClaim{}, err
	}
	if len(values) != 4 {
		return chat.IdempotencyClaim{}, fmt.Errorf("unexpected idempotency claim response")
	}
	state := chat.ClaimState(values[0])
	claim := chat.IdempotencyClaim{State: state, Token: values[1], Request: request, UserMessageID: values[2], AssistantMessageID: values[3]}
	if state == chat.ClaimNew || state == chat.ClaimFailed {
		claim.OriginalState = state
	}
	return claim, nil
}

func (a *ReliabilityAdapter) Reclaim(ctx context.Context, request chat.IdempotencyRequest) (chat.IdempotencyClaim, error) {
	token, err := randomToken()
	if err != nil {
		return chat.IdempotencyClaim{}, err
	}
	values, err := scriptStrings(ctx, a.client.Client().Eval(ctx, reclaimScript, []string{idempotencyKey(request)}, request.Fingerprint, token, milliseconds(a.idempotencyTTL)))
	if err != nil {
		return chat.IdempotencyClaim{}, err
	}
	if len(values) != 4 || values[0] != string(chat.ClaimFailed) {
		return chat.IdempotencyClaim{}, fmt.Errorf("idempotency record cannot be reclaimed")
	}
	return chat.IdempotencyClaim{State: chat.ClaimFailed, OriginalState: chat.ClaimFailed, Token: values[1], Request: request, UserMessageID: values[2], AssistantMessageID: values[3]}, nil
}

func (a *ReliabilityAdapter) AttachUserMessage(ctx context.Context, claim chat.IdempotencyClaim, userID string) error {
	return a.expectOne(ctx, attachUserScript, []string{idempotencyKey(claim.Request)}, claim.Token, userID, milliseconds(a.idempotencyTTL))
}

func (a *ReliabilityAdapter) Complete(ctx context.Context, claim chat.IdempotencyClaim, userID, assistantID string) error {
	return a.expectOne(ctx, completeScript, []string{idempotencyKey(claim.Request)}, claim.Token, userID, assistantID, milliseconds(a.idempotencyTTL))
}

func (a *ReliabilityAdapter) Fail(ctx context.Context, claim chat.IdempotencyClaim, userID string) error {
	return a.expectOne(ctx, failScript, []string{idempotencyKey(claim.Request)}, claim.Token, userID, milliseconds(a.idempotencyTTL))
}

func (a *ReliabilityAdapter) Abort(ctx context.Context, claim chat.IdempotencyClaim) error {
	if claim.Token == "" {
		return nil
	}
	_, err := a.client.Client().Eval(ctx, abortScript, []string{idempotencyKey(claim.Request)}, claim.Token, string(claim.OriginalState), claim.UserMessageID, milliseconds(a.idempotencyTTL)).Result()
	return err
}

func (a *ReliabilityAdapter) AcquireSession(ctx context.Context, sessionID string) (chat.SessionLock, bool, error) {
	token, err := randomToken()
	if err != nil {
		return chat.SessionLock{}, false, err
	}
	ok, err := a.client.Client().SetNX(ctx, sessionLockKey(sessionID), token, a.lockTTL).Result()
	if err != nil {
		return chat.SessionLock{}, false, err
	}
	return chat.SessionLock{SessionID: sessionID, Token: token}, ok, nil
}

func (a *ReliabilityAdapter) ReleaseSession(ctx context.Context, lock chat.SessionLock) error {
	if lock.Token == "" {
		return nil
	}
	_, err := a.client.Client().Eval(ctx, releaseLockScript, []string{sessionLockKey(lock.SessionID)}, lock.Token).Result()
	return err
}

func (a *ReliabilityAdapter) AllowOwner(ctx context.Context, ownerKey string, now time.Time) (bool, error) {
	minute := now.UTC().Format("200601021504")
	value, err := a.client.Client().Eval(ctx, rateLimitScript, []string{rateLimitKey(ownerKey, minute)}, "60000", strconv.Itoa(a.ownerLimit)).Int()
	if err != nil {
		return false, err
	}
	return value == 1, nil
}

// ReliabilityKeys exposes deterministic key derivation for adapter tests and
// diagnostics without leaking caller-provided key components into Redis names.
func ReliabilityKeys(request chat.IdempotencyRequest, now time.Time) (string, string, string) {
	return idempotencyKey(request), sessionLockKey(request.SessionID), rateLimitKey(request.OwnerKey, now.UTC().Format("200601021504"))
}

func (a *ReliabilityAdapter) expectOne(ctx context.Context, script string, keys []string, args ...any) error {
	value, err := a.client.Client().Eval(ctx, script, keys, args...).Int()
	if err != nil {
		return err
	}
	if value != 1 {
		return fmt.Errorf("Redis reliability operation lost ownership")
	}
	return nil
}

func idempotencyKey(request chat.IdempotencyRequest) string {
	return "idempotency:" + keyPart(request.OwnerKey) + ":" + keyPart(request.SessionID) + ":" + keyPart(request.ClientMessageID)
}
func sessionLockKey(sessionID string) string { return "lock:conversation:" + keyPart(sessionID) }
func rateLimitKey(ownerKey, minute string) string {
	return "rate_limit:owner:" + keyPart(ownerKey) + ":" + minute
}
func keyPart(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func milliseconds(value time.Duration) string { return strconv.FormatInt(value.Milliseconds(), 10) }

func randomToken() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate Redis operation token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func scriptStrings(ctx context.Context, command interface{ Result() (interface{}, error) }) ([]string, error) {
	result, err := command.Result()
	if err != nil {
		return nil, err
	}
	values, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected Redis script response %T", result)
	}
	strings := make([]string, len(values))
	for i, value := range values {
		strings[i] = fmt.Sprint(value)
	}
	return strings, nil
}
