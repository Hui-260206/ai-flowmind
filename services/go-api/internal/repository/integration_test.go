//go:build integration

package repository_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"ai-flowmind/services/go-api/internal/config"
	"ai-flowmind/services/go-api/internal/migrate"
	"ai-flowmind/services/go-api/internal/model"
	"ai-flowmind/services/go-api/internal/mysql"
	"ai-flowmind/services/go-api/internal/repository"
)

// openTestDB 连接本机 MySQL（配置从环境变量读取）并执行迁移。
// 连不上时跳过测试，避免 CI 或未启动数据库的环境失败。
func openTestDB(t *testing.T) *mysql.Client {
	t.Helper()
	port, _ := strconv.Atoi(envOr("MYSQL_PORT", "3306"))
	client, err := mysql.Open(config.MySQLConfig{
		Host:     envOr("MYSQL_HOST", "127.0.0.1"),
		Port:     port,
		Database: envOr("MYSQL_DATABASE", "flowmind_test"),
		User:     envOr("MYSQL_USER", ""),
		Password: os.Getenv("MYSQL_PASSWORD"),
		Timezone: envOr("MYSQL_TIMEZONE", "Asia/Shanghai"),
	})
	if err != nil {
		t.Skipf("skip integration test: open mysql: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := migrate.Up(client.SQLDB()); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return client
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newTestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func newSession(ownerKey string) *model.Session {
	now := time.Now()
	return &model.Session{
		ID:           newTestID(),
		OwnerKey:     ownerKey,
		Title:        "",
		ModelProfile: "default",
		Status:       model.SessionActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestMigrationIsRepeatable(t *testing.T) {
	client := openTestDB(t)
	// openTestDB 已执行过一次 Up；再执行一次必须是无害的 no-op。
	if err := migrate.Up(client.SQLDB()); err != nil {
		t.Fatalf("second migrate.Up() should be a no-op, got %v", err)
	}
}

func TestSessionCRUDAndCrossOwnerIsolation(t *testing.T) {
	client := openTestDB(t)
	sessions := repository.NewSessionRepository(client.DB())
	ctx := context.Background()

	ownerA := model.OwnerKey(newTestID())
	ownerB := model.OwnerKey(newTestID())

	s := newSession(ownerA)
	if err := sessions.Create(ctx, s); err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() { _ = sessions.SoftDelete(ctx, ownerA, s.ID) })

	got, err := sessions.GetByID(ctx, ownerA, s.ID)
	if err != nil {
		t.Fatalf("get by owner A: %v", err)
	}
	if got.ID != s.ID {
		t.Fatalf("got.ID = %s, want %s", got.ID, s.ID)
	}

	// 跨 owner 访问必须返回 ErrNotFound，而不是泄露会话存在。
	if _, err := sessions.GetByID(ctx, ownerB, s.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("cross-owner GetByID err = %v, want ErrNotFound", err)
	}

	listA, err := sessions.ListByOwner(ctx, ownerA)
	if err != nil {
		t.Fatalf("list owner A: %v", err)
	}
	if len(listA) == 0 {
		t.Fatal("owner A should see its session")
	}
	listB, err := sessions.ListByOwner(ctx, ownerB)
	if err != nil {
		t.Fatalf("list owner B: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("owner B should see no sessions, got %d", len(listB))
	}

	if err := sessions.SoftDelete(ctx, ownerA, s.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if _, err := sessions.GetByID(ctx, ownerA, s.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("after delete GetByID err = %v, want ErrNotFound", err)
	}
}

func TestAppendMessageSeqAndOrder(t *testing.T) {
	client := openTestDB(t)
	sessions := repository.NewSessionRepository(client.DB())
	messages := repository.NewMessageRepository(client.DB())
	ctx := context.Background()

	owner := model.OwnerKey(newTestID())
	s := newSession(owner)
	if err := sessions.Create(ctx, s); err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() { _ = sessions.SoftDelete(ctx, owner, s.ID) })

	clientMsgID := "client-msg-001"
	user := &model.Message{
		ID:              newTestID(),
		SessionID:       s.ID,
		Role:            model.RoleUser,
		Content:         "hi",
		Status:          model.StatusCompleted,
		ClientMessageID: &clientMsgID,
		CreatedAt:       time.Now(),
	}
	seq1, err := messages.AppendMessage(ctx, user)
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if seq1 != 1 {
		t.Fatalf("first seq = %d, want 1", seq1)
	}

	assistant := &model.Message{
		ID:        newTestID(),
		SessionID: s.ID,
		Role:      model.RoleAssistant,
		Content:   "hello",
		Status:    model.StatusCompleted,
		CreatedAt: time.Now(),
	}
	seq2, err := messages.AppendMessage(ctx, assistant)
	if err != nil {
		t.Fatalf("append assistant message: %v", err)
	}
	if seq2 != 2 {
		t.Fatalf("second seq = %d, want 2", seq2)
	}

	msgs, err := messages.ListBySession(ctx, s.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("message count = %d, want 2", len(msgs))
	}
	if msgs[0].Seq != 1 || msgs[1].Seq != 2 {
		t.Fatalf("unexpected order, seqs = [%d, %d]", msgs[0].Seq, msgs[1].Seq)
	}

	found, err := messages.GetByClientMessageID(ctx, s.ID, clientMsgID)
	if err != nil {
		t.Fatalf("get by client message id: %v", err)
	}
	if found.ID != user.ID {
		t.Fatalf("found.ID = %s, want %s", found.ID, user.ID)
	}
}

func TestOutboxCreateAndMarkPublished(t *testing.T) {
	client := openTestDB(t)
	outbox := repository.NewOutboxRepository(client.DB())
	ctx := context.Background()

	event := &model.OutboxEvent{
		EventType:     "chat.completed",
		AggregateType: "conversation",
		AggregateID:   newTestID(),
		Payload:       "{}",
		Status:        model.OutboxPending,
		CreatedAt:     time.Now(),
	}
	if err := outbox.Create(ctx, event); err != nil {
		t.Fatalf("create outbox event: %v", err)
	}
	t.Cleanup(func() {
		_ = client.DB().Exec("DELETE FROM outbox_events WHERE id = ?", event.ID).Error
	})

	pending, err := outbox.ListPending(ctx, 100)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	found := false
	for _, p := range pending {
		if p.ID == event.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("created event not found in pending list")
	}

	if err := outbox.MarkPublished(ctx, event.ID, time.Now()); err != nil {
		t.Fatalf("mark published: %v", err)
	}
}
