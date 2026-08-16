package repository

import (
	"context"
	"time"

	"ai-flowmind/services/go-api/internal/model"

	"gorm.io/gorm"
)

// OutboxRepository 定义 Outbox 事件数据访问。阶段 2 只定义接口和实现，
// 业务上阶段 7 才会真正调用。
type OutboxRepository interface {
	Create(ctx context.Context, e *model.OutboxEvent) error
	ListPending(ctx context.Context, limit int) ([]model.OutboxEvent, error)
	MarkPublished(ctx context.Context, id uint64, publishedAt time.Time) error
}

type gormOutboxRepository struct{ db *gorm.DB }

func NewOutboxRepository(db *gorm.DB) OutboxRepository { return &gormOutboxRepository{db: db} }

func (r *gormOutboxRepository) Create(ctx context.Context, e *model.OutboxEvent) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *gormOutboxRepository) ListPending(ctx context.Context, limit int) ([]model.OutboxEvent, error) {
	query := r.db.WithContext(ctx).
		Where("status = ?", model.OutboxPending).
		Order("id ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var events []model.OutboxEvent
	if err := query.Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (r *gormOutboxRepository) MarkPublished(ctx context.Context, id uint64, publishedAt time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&model.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": model.OutboxPublished, "published_at": publishedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
