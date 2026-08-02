package data

import (
	"context"

	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"
)

type cronRepo struct {
	db *gorm.DB
}

// NewCronRepo 定时任务存储。
func NewCronRepo(db *gorm.DB) biz.CronDataRepo {
	return &cronRepo{db: db}
}

func (r *cronRepo) List(ctx context.Context) ([]po.CronJob, error) {
	var list []po.CronJob
	err := r.db.WithContext(ctx).Order("id DESC").Find(&list).Error
	return list, err
}

func (r *cronRepo) ListEnabled(ctx context.Context) ([]po.CronJob, error) {
	var list []po.CronJob
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&list).Error
	return list, err
}

func (r *cronRepo) GetByID(ctx context.Context, id int64) (*po.CronJob, error) {
	var j po.CronJob
	if err := r.db.WithContext(ctx).First(&j, id).Error; err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *cronRepo) Create(ctx context.Context, j *po.CronJob) error {
	return r.db.WithContext(ctx).Create(j).Error
}

func (r *cronRepo) Update(ctx context.Context, j *po.CronJob) error {
	return r.db.WithContext(ctx).Save(j).Error
}

func (r *cronRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&po.CronJob{}, id).Error
}
