package data

import (
	"context"
	"errors"
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

// FindUserByFeishuOpenID 按飞书 open_id 查用户；不存在返回 gorm.ErrRecordNotFound。
func (r *authRepo) FindUserByFeishuOpenID(ctx context.Context, openid string) (*po.AdminUser, error) {
	var u po.AdminUser
	if err := r.db.WithContext(ctx).Where("feishu_open_id = ?", openid).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser 新建用户（飞书首登自动建号；密码登录的初始 admin 由 seed 创建）。
func (r *authRepo) CreateUser(ctx context.Context, u *po.AdminUser) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// UpdateFeishuProfile 回写飞书资料（昵称/头像/邮箱/union_id），保持与飞书侧一致。
func (r *authRepo) UpdateFeishuProfile(ctx context.Context, id int64, displayName, avatar, email, unionID string) error {
	return r.db.WithContext(ctx).Model(&po.AdminUser{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"display_name": displayName, "avatar": avatar,
			"email": email, "feishu_union_id": unionID,
		}).Error
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

func (r *authRepo) DeleteOtherSessions(ctx context.Context, userID int64, keepSessionID string) error {
	return r.db.WithContext(ctx).Where("user_id = ? AND id <> ?", userID, keepSessionID).
		Delete(&po.Session{}).Error
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

// ---- ArcheryRepo ----

type archeryRepo struct{ db *gorm.DB }

func NewArcheryRepo(db *gorm.DB) biz.ArcheryRepo { return &archeryRepo{db: db} }

func (r *archeryRepo) ListConnections(ctx context.Context) ([]po.ArcheryConnection, error) {
	var list []po.ArcheryConnection
	err := r.db.WithContext(ctx).Order("id asc").Find(&list).Error
	return list, err
}

func (r *archeryRepo) GetConnection(ctx context.Context, id int64) (*po.ArcheryConnection, error) {
	var c po.ArcheryConnection
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *archeryRepo) GetConnectionByName(ctx context.Context, name string) (*po.ArcheryConnection, error) {
	var c po.ArcheryConnection
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *archeryRepo) CreateConnection(ctx context.Context, c *po.ArcheryConnection) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *archeryRepo) UpdateConnection(ctx context.Context, c *po.ArcheryConnection) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *archeryRepo) DeleteConnection(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&po.ArcheryConnection{}, id).Error
}

// ---- Archery 实例 ----

func (r *archeryRepo) ListInstances(ctx context.Context, connectionID int64) ([]po.ArcheryInstance, error) {
	var list []po.ArcheryInstance
	err := r.db.WithContext(ctx).Where("connection_id = ?", connectionID).Order("instance_name asc").Find(&list).Error
	return list, err
}

func (r *archeryRepo) GetInstance(ctx context.Context, id int64) (*po.ArcheryInstance, error) {
	var in po.ArcheryInstance
	if err := r.db.WithContext(ctx).First(&in, id).Error; err != nil {
		return nil, err
	}
	return &in, nil
}

// UpsertInstance 按 (connection_id, instance_name) 存在则更新、不存在则新建。
func (r *archeryRepo) UpsertInstance(ctx context.Context, in *po.ArcheryInstance) error {
	var existing po.ArcheryInstance
	err := r.db.WithContext(ctx).
		Where("connection_id = ? AND instance_name = ?", in.ConnectionID, in.InstanceName).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.WithContext(ctx).Create(in).Error
	}
	if err != nil {
		return err
	}
	existing.DBType = in.DBType
	return r.db.WithContext(ctx).Save(&existing).Error
}

// DeleteInstancesNotIn 删除该连接下不在 keep 名单里的实例（同步后清理已移除的）。
func (r *archeryRepo) DeleteInstancesNotIn(ctx context.Context, connectionID int64, keep []string) error {
	q := r.db.WithContext(ctx).Where("connection_id = ?", connectionID)
	if len(keep) > 0 {
		q = q.Where("instance_name NOT IN ?", keep)
	}
	return q.Delete(&po.ArcheryInstance{}).Error
}
