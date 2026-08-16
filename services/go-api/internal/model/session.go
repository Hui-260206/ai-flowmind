package model

import "time"

// SessionStatus 是会话状态。MVP 只用 active，软删除由 DeletedAt 表达；
// 该字段为阶段 0 契约预留，枚举留待后续阶段定义。
type SessionStatus string

const (
	SessionActive SessionStatus = "active"
)

// Session 对应 chat_sessions 表。
func (Session) TableName() string { return "chat_sessions" }

type Session struct {
	ID            string        `gorm:"column:id;primaryKey"`
	OwnerKey      string        `gorm:"column:owner_key"`
	Title         string        `gorm:"column:title"`
	ModelProfile  string        `gorm:"column:model_profile"`
	Status        SessionStatus `gorm:"column:status"`
	CreatedAt     time.Time     `gorm:"column:created_at"`
	UpdatedAt     time.Time     `gorm:"column:updated_at"`
	LastMessageAt *time.Time    `gorm:"column:last_message_at"`
	DeletedAt     *time.Time    `gorm:"column:deleted_at"`
}
