package model

import "time"

// MessageRole 是可扩展枚举，禁止用 is_user 之类的布尔字段表达角色。
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// MessageStatus 是消息状态。
type MessageStatus string

const (
	StatusPending   MessageStatus = "pending"
	StatusCompleted MessageStatus = "completed"
	StatusFailed    MessageStatus = "failed"
)

// Message 对应 chat_messages 表。
func (Message) TableName() string { return "chat_messages" }

type Message struct {
	ID               string        `gorm:"column:id;primaryKey"`
	SessionID        string        `gorm:"column:session_id"`
	Seq              int64         `gorm:"column:seq"`
	Role             MessageRole   `gorm:"column:role"`
	Content          string        `gorm:"column:content"`
	Status           MessageStatus `gorm:"column:status"`
	ClientMessageID  *string       `gorm:"column:client_message_id"`
	SendOperationID  *string       `gorm:"column:send_operation_id"`
	ModelName        *string       `gorm:"column:model_name"`
	PromptTokens     *int          `gorm:"column:prompt_tokens"`
	CompletionTokens *int          `gorm:"column:completion_tokens"`
	CreatedAt        time.Time     `gorm:"column:created_at"`
}

// SendOperation records the durable recovery state for one client send. Redis
// coordinates active work, while this record preserves the exact message pair
// when a Redis command succeeds remotely but its response is lost locally.
func (SendOperation) TableName() string { return "chat_send_operations" }

type SendOperation struct {
	ID                 string    `gorm:"column:id;primaryKey"`
	OwnerKey           string    `gorm:"column:owner_key"`
	SessionID          string    `gorm:"column:session_id"`
	ClientMessageID    string    `gorm:"column:client_message_id"`
	Fingerprint        string    `gorm:"column:fingerprint"`
	Status             string    `gorm:"column:status"`
	UserMessageID      string    `gorm:"column:user_message_id"`
	AssistantMessageID string    `gorm:"column:assistant_message_id"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}
