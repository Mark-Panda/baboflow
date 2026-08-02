package data

import (
	"context"
	"errors"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

type ruleChainRepo struct{ db *gorm.DB }

func NewRuleChainRepo(db *gorm.DB) biz.RuleChainRepo { return &ruleChainRepo{db: db} }

func (r *ruleChainRepo) Create(ctx context.Context, c *po.RuleChain) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *ruleChainRepo) Update(ctx context.Context, c *po.RuleChain) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *ruleChainRepo) Get(ctx context.Context, id string) (*po.RuleChain, error) {
	var c po.RuleChain
	if err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *ruleChainRepo) List(ctx context.Context, status, keyword string, page, pageSize int) ([]po.RuleChain, int64, error) {
	var (
		list  []po.RuleChain
		total int64
	)
	q := r.db.WithContext(ctx).Model(&po.RuleChain{}).Where("deleted_at IS NULL")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR description ILIKE ?", kw, kw)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if err := q.Order("updated_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *ruleChainRepo) Delete(ctx context.Context, id string) error {
	// 软删除
	return r.db.WithContext(ctx).Model(&po.RuleChain{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", gorm.Expr("now()")).Error
}

func (r *ruleChainRepo) CreateVersion(ctx context.Context, v *po.RuleChainVersion) error {
	return r.db.WithContext(ctx).Create(v).Error
}

func (r *ruleChainRepo) ListVersions(ctx context.Context, chainID string) ([]po.RuleChainVersion, error) {
	var list []po.RuleChainVersion
	if err := r.db.WithContext(ctx).Where("chain_id = ?", chainID).
		Order("version DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *ruleChainRepo) GetVersion(ctx context.Context, chainID string, version int) (*po.RuleChainVersion, error) {
	var v po.RuleChainVersion
	if err := r.db.WithContext(ctx).
		Where("chain_id = ? AND version = ?", chainID, version).
		First(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, biz.ErrNotFound
		}
		return nil, err
	}
	return &v, nil
}

func (r *ruleChainRepo) CreateRun(ctx context.Context, run *po.ChainRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *ruleChainRepo) UpdateRun(ctx context.Context, run *po.ChainRun) error {
	return r.db.WithContext(ctx).Save(run).Error
}

func (r *ruleChainRepo) ListRuns(ctx context.Context, chainID, status string, page, pageSize int) ([]po.ChainRun, int64, error) {
	var (
		list  []po.ChainRun
		total int64
	)
	q := r.db.WithContext(ctx).Model(&po.ChainRun{})
	if chainID != "" {
		q = q.Where("chain_id = ?", chainID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *ruleChainRepo) GetRun(ctx context.Context, id int64) (*po.ChainRun, error) {
	var run po.ChainRun
	if err := r.db.WithContext(ctx).First(&run, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, biz.ErrNotFound
		}
		return nil, err
	}
	return &run, nil
}
