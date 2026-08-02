package biz

import (
	"context"
	"errors"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"

	"gorm.io/datatypes"
)

var (
	ErrReferenced = errors.New("资源被引用，无法删除")
	ErrNotFound   = errors.New("资源不存在")
)

type LLMRepo interface {
	ListProviders(ctx context.Context) ([]po.LLMProvider, error)
	GetProvider(ctx context.Context, id int64) (*po.LLMProvider, error)
	CreateProvider(ctx context.Context, p *po.LLMProvider) error
	UpdateProvider(ctx context.Context, p *po.LLMProvider) error
	DeleteProvider(ctx context.Context, id int64) error

	ListModels(ctx context.Context, providerID int64) ([]po.LLMModel, error)
	GetModel(ctx context.Context, id int64) (*po.LLMModel, error)
	CreateModel(ctx context.Context, m *po.LLMModel) error
	UpdateModel(ctx context.Context, m *po.LLMModel) error
	DeleteModel(ctx context.Context, id int64) error
	SetDefaultModel(ctx context.Context, providerID, modelID int64) error
	CountAgentByModel(ctx context.Context, modelID int64) (int64, error)
}

type LLMUsecase struct {
	repo   LLMRepo
	secret string
}

func NewLLMUsecase(repo LLMRepo, c *conf.Config) *LLMUsecase {
	return &LLMUsecase{repo: repo, secret: c.Secret}
}

// ---- Provider ----

type ProviderView struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	BaseURL      string `json:"baseUrl"`
	APIKeyMasked string `json:"apiKeyMasked"`
	Extra        datatypes.JSON `json:"extra"`
	Remark       string `json:"remark"`
	ModelCount   int    `json:"modelCount"`
}

func (uc *LLMUsecase) ListProviders(ctx context.Context) ([]ProviderView, error) {
	ps, err := uc.repo.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderView, 0, len(ps))
	for _, p := range ps {
		key, _ := conf.Decrypt(uc.secret, p.APIKeyEnc)
		out = append(out, ProviderView{
			ID: p.ID, Name: p.Name, Provider: p.Provider, BaseURL: p.BaseURL,
			APIKeyMasked: conf.Mask(key), Extra: p.Extra, Remark: p.Remark,
			ModelCount: len(p.Models),
		})
	}
	return out, nil
}

type ProviderInput struct {
	Name     string         `json:"name" binding:"required"`
	Provider string         `json:"provider"`
	BaseURL  string         `json:"baseUrl" binding:"required"`
	APIKey   string         `json:"apiKey"` // 更新时留空表示不修改
	Extra    datatypes.JSON `json:"extra"`
	Remark   string         `json:"remark"`
}

func (uc *LLMUsecase) CreateProvider(ctx context.Context, in *ProviderInput) (*po.LLMProvider, error) {
	if err := validateProviderBaseURL(in.BaseURL); err != nil {
		return nil, err
	}
	enc, err := conf.Encrypt(uc.secret, in.APIKey)
	if err != nil {
		return nil, err
	}
	if in.Provider == "" {
		in.Provider = "openai"
	}
	if in.Extra == nil {
		in.Extra = datatypes.JSON([]byte("{}"))
	}
	p := &po.LLMProvider{
		Name: in.Name, Provider: in.Provider, BaseURL: in.BaseURL,
		APIKeyEnc: enc, Extra: in.Extra, Remark: in.Remark,
	}
	if err := uc.repo.CreateProvider(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (uc *LLMUsecase) UpdateProvider(ctx context.Context, id int64, in *ProviderInput) error {
	if err := validateProviderBaseURL(in.BaseURL); err != nil {
		return err
	}
	p, err := uc.repo.GetProvider(ctx, id)
	if err != nil {
		return err
	}
	p.Name = in.Name
	p.BaseURL = in.BaseURL
	p.Remark = in.Remark
	if in.Provider != "" {
		p.Provider = in.Provider
	}
	if in.Extra != nil {
		p.Extra = in.Extra
	}
	if in.APIKey != "" {
		enc, err := conf.Encrypt(uc.secret, in.APIKey)
		if err != nil {
			return err
		}
		p.APIKeyEnc = enc
	}
	return uc.repo.UpdateProvider(ctx, p)
}

func (uc *LLMUsecase) DeleteProvider(ctx context.Context, id int64) error {
	models, err := uc.repo.ListModels(ctx, id)
	if err != nil {
		return err
	}
	for _, m := range models {
		if n, _ := uc.repo.CountAgentByModel(ctx, m.ID); n > 0 {
			return ErrReferenced
		}
	}
	return uc.repo.DeleteProvider(ctx, id)
}

// ---- Model ----

type ModelInput struct {
	Model       string         `json:"model" binding:"required"`
	Alias       string         `json:"alias"`
	Temperature float64        `json:"temperature"`
	MaxTokens   int            `json:"maxTokens"`
	IsDefault   bool           `json:"isDefault"`
	Capability  datatypes.JSON `json:"capability"`
	Enabled     *bool          `json:"enabled"`
}

func (uc *LLMUsecase) ListModels(ctx context.Context, providerID int64) ([]po.LLMModel, error) {
	return uc.repo.ListModels(ctx, providerID)
}

func (uc *LLMUsecase) CreateModels(ctx context.Context, providerID int64, ins []ModelInput) error {
	for _, in := range ins {
		cap := in.Capability
		if cap == nil {
			cap = datatypes.JSON([]byte(`{"chat":true}`))
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		mt := in.MaxTokens
		if mt == 0 {
			mt = 4096
		}
		temp := in.Temperature
		if temp == 0 {
			temp = 0.7
		}
		m := &po.LLMModel{
			ProviderID: providerID, Model: in.Model, Alias: in.Alias,
			Temperature: temp, MaxTokens: mt, IsDefault: in.IsDefault,
			Capability: cap, Enabled: enabled,
		}
		if err := uc.repo.CreateModel(ctx, m); err != nil {
			return err
		}
		if in.IsDefault {
			_ = uc.repo.SetDefaultModel(ctx, providerID, m.ID)
		}
	}
	return nil
}

func (uc *LLMUsecase) UpdateModel(ctx context.Context, modelID int64, in *ModelInput) error {
	m, err := uc.repo.GetModel(ctx, modelID)
	if err != nil {
		return err
	}
	if in.Alias != "" {
		m.Alias = in.Alias
	}
	if in.Temperature != 0 {
		m.Temperature = in.Temperature
	}
	if in.MaxTokens != 0 {
		m.MaxTokens = in.MaxTokens
	}
	if in.Capability != nil {
		m.Capability = in.Capability
	}
	if in.Enabled != nil {
		m.Enabled = *in.Enabled
	}
	if err := uc.repo.UpdateModel(ctx, m); err != nil {
		return err
	}
	if in.IsDefault {
		return uc.repo.SetDefaultModel(ctx, m.ProviderID, m.ID)
	}
	return nil
}

func (uc *LLMUsecase) DeleteModel(ctx context.Context, modelID int64) error {
	if n, _ := uc.repo.CountAgentByModel(ctx, modelID); n > 0 {
		return ErrReferenced
	}
	return uc.repo.DeleteModel(ctx, modelID)
}

func (uc *LLMUsecase) SetDefaultModel(ctx context.Context, modelID int64) error {
	m, err := uc.repo.GetModel(ctx, modelID)
	if err != nil {
		return err
	}
	return uc.repo.SetDefaultModel(ctx, m.ProviderID, m.ID)
}
