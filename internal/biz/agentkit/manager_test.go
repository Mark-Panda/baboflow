package agentkit

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/datatypes"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"
)

// ---- stubs ----

type stubAgentRepo struct{ agent *po.Agent }

func (s *stubAgentRepo) GetByKey(ctx context.Context, key string) (*po.Agent, error) {
	if s.agent != nil && s.agent.Key == key {
		return s.agent, nil
	}
	return nil, errors.New("record not found")
}
func (s *stubAgentRepo) GetByID(ctx context.Context, id int64) (*po.Agent, error) {
	return s.agent, nil
}
func (s *stubAgentRepo) ListSubAgents(ctx context.Context, parentID int64) ([]po.AgentSubAgent, error) {
	return nil, nil
}

type stubSkillRepo struct{}

func (s *stubSkillRepo) ListByIDs(ctx context.Context, ids []int64) ([]po.Skill, error) {
	return nil, nil
}
func (s *stubSkillRepo) GetByName(ctx context.Context, name string) (*po.Skill, error) {
	return nil, errors.New("not found")
}

type stubLLMResolver struct {
	provider *po.LLMProvider
	model    *po.LLMModel
	err      error
}

func (s *stubLLMResolver) ResolveForAgent(ctx context.Context, modelID *int64) (*po.LLMProvider, *po.LLMModel, error) {
	if s.err != nil {
		return nil, nil, s.err
	}
	return s.provider, s.model, nil
}

func testConfig() *conf.Config {
	return &conf.Config{Secret: "baboflow-dev-secret-32bytes-pad!"}
}

func encKey(t *testing.T, c *conf.Config, plain string) string {
	t.Helper()
	enc, err := conf.Encrypt(c.Secret, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return enc
}

// ---- tests ----

// 验证 Manager.Get → build 全流程：构造 ChatModel + 内置工具装配 + 缓存命中。
func TestManagerBuildsAndCachesAgent(t *testing.T) {
	c := testConfig()
	agent := &po.Agent{
		ID: 1, Key: "agent-general", Name: "通用助手",
		Instruction:  "测试指令",
		BuiltinTools: datatypes.JSON([]byte(`["bash","read"]`)),
		SkillIDs:     datatypes.JSON([]byte(`[]`)),
		McpIDs:       datatypes.JSON([]byte(`[]`)),
		Enabled:      true,
		UpdatedAt:    time.Now(),
	}
	resolver := &stubLLMResolver{
		provider: &po.LLMProvider{ID: 1, BaseURL: "https://api.openai.com/v1", APIKeyEnc: encKey(t, c, "sk-test")},
		model:    &po.LLMModel{ID: 1, Model: "gpt-4o-mini", MaxTokens: 1024},
	}
	mgr := NewManager(&stubAgentRepo{agent}, &stubSkillRepo{}, resolver, NewModelFactory(c), NewBuiltinTools(t.TempDir(), nil))

	ag, err := mgr.Get(context.Background(), "agent-general")
	if err != nil {
		t.Fatalf("Get build failed: %v", err)
	}
	if ag == nil {
		t.Fatal("expect non-nil agent")
	}

	// 第二次 Get 应命中缓存（不重建）
	ag2, err := mgr.Get(context.Background(), "agent-general")
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	// 缓存的应是同一个实例
	if ag != ag2 {
		t.Fatal("expect cached same agent instance")
	}

	// Invalidate 后重建为新实例
	mgr.Invalidate("agent-general")
	ag3, err := mgr.Get(context.Background(), "agent-general")
	if err != nil {
		t.Fatalf("Get after invalidate failed: %v", err)
	}
	if ag3 == ag {
		t.Fatal("expect rebuilt instance after invalidate")
	}
}

// 配置变更（UpdatedAt 变化）应触发重建而非用旧缓存。
func TestManagerRebuildsOnConfigChange(t *testing.T) {
	c := testConfig()
	agent := &po.Agent{
		ID: 1, Key: "k", Name: "n", Instruction: "i",
		BuiltinTools: datatypes.JSON([]byte(`["read"]`)),
		SkillIDs:     datatypes.JSON([]byte(`[]`)),
		McpIDs:       datatypes.JSON([]byte(`[]`)),
		Enabled:      true,
		UpdatedAt:    time.Now(),
	}
	resolver := &stubLLMResolver{
		provider: &po.LLMProvider{ID: 1, BaseURL: "https://x", APIKeyEnc: encKey(t, c, "sk")},
		model:    &po.LLMModel{ID: 1, Model: "m"},
	}
	repo := &stubAgentRepo{agent}
	mgr := NewManager(repo, &stubSkillRepo{}, resolver, NewModelFactory(c), NewBuiltinTools(t.TempDir(), nil))

	a1, _ := mgr.Get(context.Background(), "k")
	// 模拟配置更新：UpdatedAt 前进
	agent.UpdatedAt = agent.UpdatedAt.Add(time.Second)
	a2, _ := mgr.Get(context.Background(), "k")
	if a1 == a2 {
		t.Fatal("expect rebuild when UpdatedAt changed")
	}
}

// 停用的 agent 应报错。
func TestManagerRejectsDisabledAgent(t *testing.T) {
	c := testConfig()
	agent := &po.Agent{Key: "off", Enabled: false, BuiltinTools: datatypes.JSON([]byte(`[]`)), SkillIDs: datatypes.JSON([]byte(`[]`)), McpIDs: datatypes.JSON([]byte(`[]`))}
	mgr := NewManager(&stubAgentRepo{agent}, &stubSkillRepo{}, &stubLLMResolver{}, NewModelFactory(c), NewBuiltinTools(t.TempDir(), nil))
	if _, err := mgr.Get(context.Background(), "off"); err == nil {
		t.Fatal("expect disabled error")
	}
}

// 无可用模型时应报清晰错误。
func TestManagerPropagatesResolveError(t *testing.T) {
	c := testConfig()
	agent := &po.Agent{Key: "k", Enabled: true, BuiltinTools: datatypes.JSON([]byte(`[]`)), SkillIDs: datatypes.JSON([]byte(`[]`)), McpIDs: datatypes.JSON([]byte(`[]`))}
	resolver := &stubLLMResolver{err: errors.New("未找到可用 LLM 模型")}
	mgr := NewManager(&stubAgentRepo{agent}, &stubSkillRepo{}, resolver, NewModelFactory(c), NewBuiltinTools(t.TempDir(), nil))
	if _, err := mgr.Get(context.Background(), "k"); err == nil {
		t.Fatal("expect resolve error")
	}
}
