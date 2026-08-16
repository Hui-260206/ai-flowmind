package repository

import (
	"context"
	"errors"
	"time"

	"ai-flowmind/services/go-api/internal/model"

	"gorm.io/gorm"
)

// SessionRepository 定义会话数据访问。所有查询都必须带 owner_key 条件。
type SessionRepository interface {
	// Create 新建会话；调用方负责填好 OwnerKey、Title、ModelProfile 与时间戳。
	Create(ctx context.Context, s *model.Session) error

	// GetByID 按 id + owner_key 取会话；不存在或不属于该 owner 都返回 ErrNotFound。
	GetByID(ctx context.Context, ownerKey, id string) (*model.Session, error)

	// ListByOwner 返回该 owner 的会话，按 updated_at 倒序。
	ListByOwner(ctx context.Context, ownerKey string) ([]model.Session, error)

	// UpdateTitleAndTime 发消息后更新标题与最近消息时间；带 owner_key 条件。
	UpdateTitleAndTime(ctx context.Context, ownerKey, id, title string, lastMessageAt time.Time) error

	// SoftDelete 软删除：置 deleted_at，不物理删除。
	SoftDelete(ctx context.Context, ownerKey, id string) error
}

type gormSessionRepository struct{ db *gorm.DB }

func NewSessionRepository(db *gorm.DB) SessionRepository { return &gormSessionRepository{db: db} }

func (r *gormSessionRepository) Create(ctx context.Context, s *model.Session) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *gormSessionRepository) GetByID(ctx context.Context, ownerKey, id string) (*model.Session, error) {
	var s model.Session
	err := r.db.WithContext(ctx).
		Where("id = ? AND owner_key = ? AND deleted_at IS NULL", id, ownerKey).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *gormSessionRepository) ListByOwner(ctx context.Context, ownerKey string) ([]model.Session, error) {
	var sessions []model.Session
	err := r.db.WithContext(ctx).
		Where("owner_key = ? AND deleted_at IS NULL", ownerKey).
		Order("updated_at DESC").
		Find(&sessions).Error
	return sessions, err
}

func (r *gormSessionRepository) UpdateTitleAndTime(ctx context.Context, ownerKey, id, title string, lastMessageAt time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&model.Session{}).
		Where("id = ? AND owner_key = ? AND deleted_at IS NULL", id, ownerKey).
		Updates(map[string]any{
			"title":           title,
			"last_message_at": lastMessageAt,
			"updated_at":      time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormSessionRepository) SoftDelete(ctx context.Context, ownerKey, id string) error {
	res := r.db.WithContext(ctx).
		Model(&model.Session{}).
		Where("id = ? AND owner_key = ? AND deleted_at IS NULL", id, ownerKey).
		Update("deleted_at", time.Now())
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
