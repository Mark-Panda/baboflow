package data

import (
	"context"

	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"
)

type auditRepo struct {
	db *gorm.DB
}

// NewAuditRepo 审计日志存储。
func NewAuditRepo(db *gorm.DB) biz.AuditDataRepo {
	return &auditRepo{db: db}
}

func (r *auditRepo) Create(ctx context.Context, e *po.AuditLog) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *auditRepo) List(ctx context.Context, action string, userID *int64, page, pageSize int) ([]po.AuditLog, int64, error) {
	q := r.db.WithContext(ctx).Model(&po.AuditLog{})
	if action != "" {
		q = q.Where("action = ?", action)
	}
	if userID != nil {
		q = q.Where("user_id = ?", *userID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []po.AuditLog
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}
