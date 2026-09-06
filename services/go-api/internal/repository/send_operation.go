package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ai-flowmind/services/go-api/internal/model"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const (
	SendOperationProcessing = "processing"
	SendOperationFailed     = "failed"
	SendOperationCompleted  = "completed"
)

// SendOperationRepository persists recovery information that must outlive
// Redis availability. All reads are constrained by the anonymous owner.
type SendOperationRepository interface {
	Get(context.Context, string, string, string) (*model.SendOperation, error)
	Create(context.Context, *model.SendOperation) error
	SetUserMessage(context.Context, string, string, string, string) error
	SetAssistantMessage(context.Context, string, string, string, string) error
	MarkFailed(context.Context, string, string, string) error
	MarkCompleted(context.Context, string, string, string) error
}

type gormSendOperationRepository struct{ db *gorm.DB }

func NewSendOperationRepository(db *gorm.DB) SendOperationRepository {
	return &gormSendOperationRepository{db: db}
}

func (r *gormSendOperationRepository) Get(ctx context.Context, ownerKey, sessionID, clientMessageID string) (*model.SendOperation, error) {
	var operation model.SendOperation
	err := r.db.WithContext(ctx).Where("owner_key = ? AND session_id = ? AND client_message_id = ?", ownerKey, sessionID, clientMessageID).First(&operation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &operation, nil
}

func (r *gormSendOperationRepository) Create(ctx context.Context, operation *model.SendOperation) error {
	if err := r.db.WithContext(ctx).Create(operation).Error; err != nil {
		var mysqlError *drivermysql.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			return fmt.Errorf("%w: %v", ErrDuplicateClientMessageID, err)
		}
		return err
	}
	return nil
}

func (r *gormSendOperationRepository) SetUserMessage(ctx context.Context, ownerKey, sessionID, clientMessageID, messageID string) error {
	return r.update(ctx, ownerKey, sessionID, clientMessageID, map[string]any{"user_message_id": messageID})
}

func (r *gormSendOperationRepository) SetAssistantMessage(ctx context.Context, ownerKey, sessionID, clientMessageID, messageID string) error {
	return r.update(ctx, ownerKey, sessionID, clientMessageID, map[string]any{"assistant_message_id": messageID})
}

func (r *gormSendOperationRepository) MarkFailed(ctx context.Context, ownerKey, sessionID, clientMessageID string) error {
	return r.update(ctx, ownerKey, sessionID, clientMessageID, map[string]any{"status": SendOperationFailed})
}

func (r *gormSendOperationRepository) MarkCompleted(ctx context.Context, ownerKey, sessionID, clientMessageID string) error {
	return r.update(ctx, ownerKey, sessionID, clientMessageID, map[string]any{"status": SendOperationCompleted})
}

func (r *gormSendOperationRepository) update(ctx context.Context, ownerKey, sessionID, clientMessageID string, values map[string]any) error {
	values["updated_at"] = time.Now()
	result := r.db.WithContext(ctx).Model(&model.SendOperation{}).Where("owner_key = ? AND session_id = ? AND client_message_id = ?", ownerKey, sessionID, clientMessageID).Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
