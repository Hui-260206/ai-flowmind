package model

import "time"

// OutboxStatus 是 Outbox 事件的状态。表结构阶段 2 建好，业务逻辑阶段 7 才使用。
type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "pending"
	OutboxPublished OutboxStatus = "published"
	OutboxFailed    OutboxStatus = "failed"
)

// OutboxEvent 对应 outbox_events 表。
func (OutboxEvent) TableName() string { return "outbox_events" }

type OutboxEvent struct {
	ID            uint64       `gorm:"column:id;primaryKey;autoIncrement"`
	EventType     string       `gorm:"column:event_type"`
	AggregateType string       `gorm:"column:aggregate_type"`
	AggregateID   string       `gorm:"column:aggregate_id"`
	Payload       string       `gorm:"column:payload"` // JSON 字符串
	Status        OutboxStatus `gorm:"column:status"`
	RetryCount    int          `gorm:"column:retry_count"`
	NextRetryAt   *time.Time   `gorm:"column:next_retry_at"`
	CreatedAt     time.Time    `gorm:"column:created_at"`
	PublishedAt   *time.Time   `gorm:"column:published_at"`
}
