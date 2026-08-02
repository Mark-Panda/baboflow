package data

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"baboflow/internal/biz"
	"baboflow/internal/data/po"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---- AgentDataRepo（biz.AgentDataRepo）----

type agentDataRepo struct{ db *gorm.DB }

func NewAgentDataRepo(db *gorm.DB) biz.AgentDataRepo { return &agentDataRepo{db: db} }

func (r *agentDataRepo) ListAgents(ctx context.Context, keyword string) ([]po.Agent, error) {
	var list []po.Agent
	q := r.db.WithContext(ctx).Order("is_builtin desc, id asc")
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name ILIKE ? OR \"key\" ILIKE ?", like, like)
	}
	err := q.Find(&list).Error
	return list, err
}

func (r *agentDataRepo) GetAgentByKey(ctx context.Context, key string) (*po.Agent, error) {
	var a po.Agent
	if err := r.db.WithContext(ctx).Where(`"key" = ?`, key).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *agentDataRepo) CreateAgent(ctx context.Context, a *po.Agent) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *agentDataRepo) UpdateAgent(ctx context.Context, a *po.Agent) error {
	return r.db.WithContext(ctx).Save(a).Error
}

func (r *agentDataRepo) DeleteAgent(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("parent_id = ? OR child_id = ?", id, id).Delete(&po.AgentSubAgent{}).Error; err != nil {
			return err
		}
		return tx.Delete(&po.Agent{}, id).Error
	})
}

func (r *agentDataRepo) SetSubAgents(ctx context.Context, parentID int64, childIDs []int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("parent_id = ?", parentID).Delete(&po.AgentSubAgent{}).Error; err != nil {
			return err
		}
		for _, cid := range childIDs {
			if cid == parentID {
				continue
			}
			if err := tx.Create(&po.AgentSubAgent{ParentID: parentID, ChildID: cid}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *agentDataRepo) ListSubAgentIDs(ctx context.Context, parentID int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&po.AgentSubAgent{}).
		Where("parent_id = ?", parentID).Pluck("child_id", &ids).Error
	return ids, err
}

func (r *agentDataRepo) CreateSession(ctx context.Context, s *po.AgentSession) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *agentDataRepo) GetSession(ctx context.Context, id string) (*po.AgentSession, error) {
	var s po.AgentSession
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *agentDataRepo) ListSessions(ctx context.Context, agentKey string, userID int64) ([]po.AgentSession, error) {
	var list []po.AgentSession
	err := r.db.WithContext(ctx).
		Where("agent_key = ? AND user_id = ?", agentKey, userID).
		Order("updated_at desc").Find(&list).Error
	return list, err
}

func (r *agentDataRepo) UpdateSessionTitle(ctx context.Context, id, title string) error {
	return r.db.WithContext(ctx).Model(&po.AgentSession{}).Where("id = ?", id).
		Updates(map[string]interface{}{"title": title, "updated_at": gorm.Expr("now()")}).Error
}

func (r *agentDataRepo) DeleteSession(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&po.AgentMessage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&po.AgentSession{}).Error
	})
}

func (r *agentDataRepo) CreateMessage(ctx context.Context, m *po.AgentMessage) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *agentDataRepo) ListMessages(ctx context.Context, sessionID string, limit int) ([]po.AgentMessage, error) {
	var list []po.AgentMessage
	if limit <= 0 {
		limit = 200
	}
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).
		Order("id asc").Limit(limit).Find(&list).Error
	return list, err
}

func (r *agentDataRepo) CreateAsset(ctx context.Context, a *po.Asset) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *agentDataRepo) GetAsset(ctx context.Context, id int64) (*po.Asset, error) {
	var a po.Asset
	if err := r.db.WithContext(ctx).First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// ---- AssetStore（biz.AssetStore，本地落盘）----

type localAssetStore struct{ root string }

func NewLocalAssetStore(workspaceRoot string) biz.AssetStore {
	abs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		abs = workspaceRoot
	}
	return &localAssetStore{root: abs}
}

// Save 把附件写到 workspace/_assets/<sessionID>/<uuid>-<name>，返回相对 root 的路径。
func (s *localAssetStore) Save(sessionID, name string, data []byte) (string, error) {
	dir := filepath.Join(s.root, "_assets", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	fname := uuid.NewString() + "-" + filepath.Base(name)
	full := filepath.Join(dir, fname)
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.root, full)
	if err != nil {
		return "", err
	}
	return rel, nil
}

func (s *localAssetStore) Read(relPath string) ([]byte, error) {
	full := filepath.Join(s.root, filepath.Clean(relPath))
	// 防越界：必须以 root 为前缀（按路径边界，而非长度比较）。
	root := filepath.Clean(s.root)
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return nil, fmt.Errorf("非法路径")
	}
	return os.ReadFile(full)
}

func (s *localAssetStore) DeleteBySession(sessionID string) error {
	dir := filepath.Join(s.root, "_assets", sessionID)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	// 同时清理 agent 工作区目录（按 sessionID 命名的）
	return os.RemoveAll(filepath.Join(s.root, sessionID))
}

// ---- LLMResolver（agentkit.LLMResolver）----

type llmResolver struct{ db *gorm.DB }

func NewLLMResolver(db *gorm.DB) *llmResolver { return &llmResolver{db: db} }

// ResolveForAgent 优先用 agent 指定的模型；未指定则回退到全局默认模型（任一 provider 的 is_default）。
func (r *llmResolver) ResolveForAgent(ctx context.Context, modelID *int64) (*po.LLMProvider, *po.LLMModel, error) {
	var m po.LLMModel
	var err error
	if modelID != nil {
		err = r.db.WithContext(ctx).First(&m, *modelID).Error
	} else {
		err = r.db.WithContext(ctx).Where("is_default = ? AND enabled = ?", true, true).
			Order("id asc").First(&m).Error
	}
	if err != nil {
		return nil, nil, fmt.Errorf("未找到可用 LLM 模型（请先在 LLM 配置中设置默认模型）: %w", err)
	}
	var p po.LLMProvider
	if err := r.db.WithContext(ctx).First(&p, m.ProviderID).Error; err != nil {
		return nil, nil, fmt.Errorf("模型所属 provider 不存在: %w", err)
	}
	return &p, &m, nil
}
