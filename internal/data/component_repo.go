package data

import (
	"context"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type componentRepo struct{ db *gorm.DB }

func NewComponentRepo(db *gorm.DB) biz.ComponentRepo { return &componentRepo{db: db} }

func (r *componentRepo) ListAll(ctx context.Context) ([]po.ComponentMeta, error) {
	var list []po.ComponentMeta
	err := r.db.WithContext(ctx).Find(&list).Error
	return list, err
}

func (r *componentRepo) Upsert(ctx context.Context, m *po.ComponentMeta) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "type"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "category", "description", "config_schema", "example", "fingerprint", "updated_at",
		}),
	}).Create(m).Error
}

// MarkMissing 删除注册表中已不存在的组件（保留 keepTypes）。
func (r *componentRepo) MarkMissing(ctx context.Context, keepTypes []string) (int64, error) {
	if len(keepTypes) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where(`"type" NOT IN ?`, keepTypes).Delete(&po.ComponentMeta{})
	return res.RowsAffected, res.Error
}

func (r *componentRepo) SearchKeyword(ctx context.Context, category, keyword string) ([]po.ComponentMeta, error) {
	q := r.db.WithContext(ctx).Model(&po.ComponentMeta{})
	if category != "" {
		q = q.Where("category = ?", category)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where(`name ILIKE ? OR description ILIKE ? OR "type" ILIKE ?`, like, like, like)
	}
	var list []po.ComponentMeta
	err := q.Order(`category asc, "type" asc`).Find(&list).Error
	return list, err
}
