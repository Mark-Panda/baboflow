package data

import (
	"context"
	"time"

	"baboflow/internal/biz/agentkit"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

// 编译期接口断言：skillRepo 同时满足 agentkit 与 biz 两侧接口。
var (
	_ agentkit.SkillRepo = (*skillRepo)(nil)
)

// ---- AgentRepo（agentkit.AgentRepo）----

type agentRepo struct{ db *gorm.DB }

func NewAgentRepo(db *gorm.DB) agentkit.AgentRepo { return &agentRepo{db: db} }

func (r *agentRepo) GetByKey(ctx context.Context, key string) (*po.Agent, error) {
	var a po.Agent
	if err := r.db.WithContext(ctx).Where(`"key" = ?`, key).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *agentRepo) GetByID(ctx context.Context, id int64) (*po.Agent, error) {
	var a po.Agent
	if err := r.db.WithContext(ctx).First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *agentRepo) ListSubAgents(ctx context.Context, parentID int64) ([]po.AgentSubAgent, error) {
	var list []po.AgentSubAgent
	err := r.db.WithContext(ctx).Where("parent_id = ?", parentID).Find(&list).Error
	return list, err
}

// ---- SkillRepo（agentkit.SkillRepo + biz.SkillDataRepo）----

type skillRepo struct{ db *gorm.DB }

func NewSkillRepo(db *gorm.DB) *skillRepo { return &skillRepo{db: db} }

func (r *skillRepo) ListByIDs(ctx context.Context, ids []int64) ([]po.Skill, error) {
	var list []po.Skill
	err := r.db.WithContext(ctx).Where("id IN ? AND deleted_at IS NULL", ids).Find(&list).Error
	return list, err
}

func (r *skillRepo) GetByName(ctx context.Context, name string) (*po.Skill, error) {
	var s po.Skill
	if err := r.db.WithContext(ctx).Where("name = ? AND deleted_at IS NULL", name).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// ---- biz.SkillDataRepo ----

func (r *skillRepo) List(ctx context.Context, source, keyword string) ([]po.Skill, error) {
	var list []po.Skill
	q := r.db.WithContext(ctx).Where("deleted_at IS NULL").Order("id desc")
	if source != "" {
		q = q.Where("source = ?", source)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR description ILIKE ?", like, like)
	}
	err := q.Find(&list).Error
	return list, err
}

func (r *skillRepo) GetByID(ctx context.Context, id int64) (*po.Skill, error) {
	var s po.Skill
	if err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *skillRepo) Create(ctx context.Context, s *po.Skill) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *skillRepo) Update(ctx context.Context, s *po.Skill) error {
	return r.db.WithContext(ctx).Save(s).Error
}

func (r *skillRepo) Delete(ctx context.Context, id int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&po.Skill{}).Where("id = ?", id).
		Update("deleted_at", now).Error
}
