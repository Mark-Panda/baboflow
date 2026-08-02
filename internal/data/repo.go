package data

import (
	"context"
	"time"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"

	"gorm.io/gorm"
)

// ---- AuthRepo ----

type authRepo struct{ db *gorm.DB }

func NewAuthRepo(db *gorm.DB) biz.AuthRepo { return &authRepo{db: db} }

func (r *authRepo) FindUserByUsername(ctx context.Context, username string) (*po.AdminUser, error) {
	var u po.AdminUser
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *authRepo) FindUserByID(ctx context.Context, id int64) (*po.AdminUser, error) {
	var u po.AdminUser
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *authRepo) UpdateUserPassword(ctx context.Context, id int64, hash string, mustChange bool) error {
	return r.db.WithContext(ctx).Model(&po.AdminUser{}).Where("id = ?", id).
		Updates(map[string]interface{}{"password_hash": hash, "must_change_pwd": mustChange}).Error
}

func (r *authRepo) TouchLastLogin(ctx context.Context, id int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&po.AdminUser{}).Where("id = ?", id).
		Update("last_login_at", now).Error
}

func (r *authRepo) CreateSession(ctx context.Context, s *po.Session) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *authRepo) FindSession(ctx context.Context, id string) (*po.Session, error) {
	var s po.Session
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *authRepo) TouchSession(ctx context.Context, id string, expiresAt time.Time) error {
	return r.db.WithContext(ctx).Model(&po.Session{}).Where("id = ?", id).
		Update("expires_at", expiresAt).Error
}

func (r *authRepo) DeleteSession(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&po.Session{}).Error
}

// ---- LLMRepo ----

type llmRepo struct{ db *gorm.DB }

func NewLLMRepo(db *gorm.DB) biz.LLMRepo { return &llmRepo{db: db} }

func (r *llmRepo) ListProviders(ctx context.Context) ([]po.LLMProvider, error) {
	var list []po.LLMProvider
	err := r.db.WithContext(ctx).Preload("Models").Order("id asc").Find(&list).Error
	return list, err
}

func (r *llmRepo) GetProvider(ctx context.Context, id int64) (*po.LLMProvider, error) {
	var p po.LLMProvider
	if err := r.db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *llmRepo) CreateProvider(ctx context.Context, p *po.LLMProvider) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *llmRepo) UpdateProvider(ctx context.Context, p *po.LLMProvider) error {
	return r.db.WithContext(ctx).Save(p).Error
}

func (r *llmRepo) DeleteProvider(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_id = ?", id).Delete(&po.LLMModel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&po.LLMProvider{}, id).Error
	})
}

func (r *llmRepo) ListModels(ctx context.Context, providerID int64) ([]po.LLMModel, error) {
	var list []po.LLMModel
	err := r.db.WithContext(ctx).Where("provider_id = ?", providerID).Order("id asc").Find(&list).Error
	return list, err
}

func (r *llmRepo) GetModel(ctx context.Context, id int64) (*po.LLMModel, error) {
	var m po.LLMModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *llmRepo) CreateModel(ctx context.Context, m *po.LLMModel) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *llmRepo) UpdateModel(ctx context.Context, m *po.LLMModel) error {
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *llmRepo) DeleteModel(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&po.LLMModel{}, id).Error
}

func (r *llmRepo) SetDefaultModel(ctx context.Context, providerID, modelID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&po.LLMModel{}).Where("provider_id = ?", providerID).
			Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&po.LLMModel{}).Where("id = ?", modelID).
			Update("is_default", true).Error
	})
}

func (r *llmRepo) CountAgentByModel(ctx context.Context, modelID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&po.Agent{}).Where("llm_model_id = ?", modelID).Count(&n).Error
	return n, err
}
