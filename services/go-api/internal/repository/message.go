package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"ai-flowmind/services/go-api/internal/model"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MessageRepository 定义消息数据访问。
type MessageRepository interface {
	// AppendMessage 在单个事务内完成：锁定会话行 → 计算 seq → 写入消息。
	// 传入的 m.Seq 会被忽略，返回实际分配的 seq（同时写回 m.Seq）。
	AppendMessage(ctx context.Context, m *model.Message) (int64, error)

	// ListBySession 按 seq 升序返回消息，limit <= 0 表示不限制。
	ListBySession(ctx context.Context, sessionID string, limit int) ([]model.Message, error)

	// GetByClientMessageID 幂等查询：同会话内按 client_message_id 找消息。
	GetByClientMessageID(ctx context.Context, sessionID, clientMessageID string) (*model.Message, error)

	// GetMessageByID 在已知会话范围内读取一条精确消息，避免跨会话重放消息。
	GetMessageByID(ctx context.Context, sessionID, id string) (*model.Message, error)

	// GetAssistantByOperation finds the durable assistant result produced for a
	// send operation when an update of the operation record was interrupted.
	GetAssistantByOperation(ctx context.Context, sessionID, operationID string) (*model.Message, error)
}

type gormMessageRepository struct{ db *gorm.DB }

func NewMessageRepository(db *gorm.DB) MessageRepository { return &gormMessageRepository{db: db} }

func (r *gormMessageRepository) AppendMessage(ctx context.Context, m *model.Message) (int64, error) {
	var seq int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 锁住会话行，使同一会话的消息写入串行化。
		var session model.Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", m.SessionID).
			First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		// 2. 锁内计算下一个 seq（MAX+1）。
		var max sql.NullInt64
		row := tx.Model(&model.Message{}).
			Where("session_id = ?", m.SessionID).
			Select("MAX(seq)").Row()
		if err := row.Scan(&max); err != nil {
			return err
		}
		if max.Valid {
			seq = max.Int64 + 1
		} else {
			seq = 1
		}

		// 3. 写入消息。uq_messages_session_seq 唯一索引是并发下的最后防线。
		m.Seq = seq
		return tx.Create(m).Error
	})
	if err != nil {
		if isDuplicateClientMessageID(err, m.ClientMessageID != nil) {
			return 0, ErrDuplicateClientMessageID
		}
		return 0, err
	}
	return seq, nil
}

func isDuplicateClientMessageID(err error, hasClientMessageID bool) bool {
	if !hasClientMessageID {
		return false
	}
	var mysqlError *drivermysql.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1062 &&
		strings.Contains(mysqlError.Message, "uq_messages_session_client")
}

func (r *gormMessageRepository) ListBySession(ctx context.Context, sessionID string, limit int) ([]model.Message, error) {
	query := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("seq ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var msgs []model.Message
	if err := query.Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

func (r *gormMessageRepository) GetByClientMessageID(ctx context.Context, sessionID, clientMessageID string) (*model.Message, error) {
	var m model.Message
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND client_message_id = ?", sessionID, clientMessageID).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *gormMessageRepository) GetMessageByID(ctx context.Context, sessionID, id string) (*model.Message, error) {
	var m model.Message
	err := r.db.WithContext(ctx).Where("session_id = ? AND id = ?", sessionID, id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *gormMessageRepository) GetAssistantByOperation(ctx context.Context, sessionID, operationID string) (*model.Message, error) {
	var m model.Message
	err := r.db.WithContext(ctx).Where("session_id = ? AND send_operation_id = ? AND role = ?", sessionID, operationID, model.RoleAssistant).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
