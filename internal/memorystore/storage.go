package memorystore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CoolBanHub/aggo/memory/builtin/storage"
	"gorm.io/gorm"
)

// PostgresStorage adapts aggo's SQL storage to PostgreSQL.
// aggo v0.3.2 declares the conversation embedding column as "blob", which
// PostgreSQL does not support; this wrapper keeps the storage implementation
// while providing PostgreSQL-compatible migrations.
type PostgresStorage struct {
	*storage.SQLStore
	db *gorm.DB
}

func NewPostgresStorage(db *gorm.DB) (*PostgresStorage, error) {
	store, err := storage.NewGormStorage(db)
	if err != nil {
		return nil, err
	}
	return &PostgresStorage{SQLStore: store, db: db}, nil
}

func (s *PostgresStorage) AutoMigrate() error {
	models := []struct {
		name  string
		model any
	}{
		{"aggo_mem_user_memories", &storage.UserMemoryModel{}},
		{"aggo_mem_session_summaries", &storage.SessionSummaryModel{}},
		{"aggo_mem_conversation_messages", &postgresConversationMessageModel{}},
		{"aggo_mem_user_memory_events", &storage.UserMemoryEventModel{}},
	}
	for _, item := range models {
		if err := s.db.Table(item.name).AutoMigrate(item.model); err != nil {
			return fmt.Errorf("migrate %s: %w", item.name, err)
		}
	}
	return nil
}

// DeleteSessionData 删除指定用户会话的对话消息和摘要。
// 用户级长期记忆及用户级事件不随单个会话删除。
func (s *PostgresStorage) DeleteSessionData(ctx context.Context, userID, sessionID string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("userID 不能为空")
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("sessionID 不能为空")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("aggo_mem_conversation_messages").
			Where("user_id = ? AND session_id = ?", userID, sessionID).
			Delete(&postgresConversationMessageModel{}).Error; err != nil {
			return fmt.Errorf("删除记忆消息失败: %w", err)
		}
		if err := tx.Table("aggo_mem_session_summaries").
			Where("user_id = ? AND session_id = ?", userID, sessionID).
			Delete(&storage.SessionSummaryModel{}).Error; err != nil {
			return fmt.Errorf("删除记忆摘要失败: %w", err)
		}
		return nil
	})
}

// Close is a no-op because the application owns the shared DB connection.
func (s *PostgresStorage) Close() error { return nil }

type postgresConversationMessageModel struct {
	ID           string               `gorm:"primaryKey;size:255"`
	SessionID    string               `gorm:"size:255;not null;index:idx_session_user"`
	UserID       string               `gorm:"size:255;not null;index:idx_session_user;index:idx_user_session"`
	Role         string               `gorm:"size:50;not null"`
	Content      string               `gorm:"type:text"`
	Parts        storage.MessageParts `gorm:"type:text"`
	Embedding    []byte               `gorm:"type:bytea"`
	EmbeddingDim int
	CreatedAt    time.Time `gorm:"autoCreateTime;index:idx_user_session"`
}
