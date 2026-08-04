package agentkit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	aggoagent "github.com/CoolBanHub/aggo/agent"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"gorm.io/datatypes"

	"baboflow/internal/data/po"
)

// AgentRepo 供 manager 读取 Agent 配置的最小接口。
type AgentRepo interface {
	GetByKey(ctx context.Context, key string) (*po.Agent, error)
	GetByID(ctx context.Context, id int64) (*po.Agent, error)
	ListSubAgents(ctx context.Context, parentID int64) ([]po.AgentSubAgent, error)
}

// LLMResolver 把 agent.llm_model_id 解析为 provider+model（默认回退默认模型）。
type LLMResolver interface {
	ResolveForAgent(ctx context.Context, modelID *int64) (*po.LLMProvider, *po.LLMModel, error)
}

// Manager 按 agent key 构建并缓存可运行的 TypedAgent。
// 缓存以 agent.UpdatedAt 为版本，配置变更即失效重建。
type Manager struct {
	agents  AgentRepo
	skills  SkillRepo
	llm     LLMResolver
	factory *ModelFactory
	builtin *BuiltinTools
	extra   ExtraToolFactory
	// ensureSkillDir 含包技能落盘回调（biz 注入），供 SkillBackend.Get 接通 BaseDirectory。
	ensureSkillDir EnsureSkillDirFunc
	maxStep int
	mu      sync.RWMutex
	cache   map[string]*cachedAgent
}

// ExtraToolFactory 让上层（M6）注入 rulechain/skill/mcp 等平台工具。
// 返回的工具会追加到 agent 工具表。sessionID 用于工作区/权限上下文。
type ExtraToolFactory func(ctx context.Context, sessionID string, a *po.Agent) ([]tool.BaseTool, error)

type cachedAgent struct {
	updatedAt int64
	agent     adk.TypedAgent[*schema.AgenticMessage]
}

func NewManager(
	agents AgentRepo,
	skills SkillRepo,
	llm LLMResolver,
	factory *ModelFactory,
	builtin *BuiltinTools,
) *Manager {
	return &Manager{
		agents:  agents,
		skills:  skills,
		llm:     llm,
		factory: factory,
		builtin: builtin,
		maxStep: 16,
		cache:   map[string]*cachedAgent{},
	}
}

// SetExtraToolFactory 注入平台工具（rulechain_validate/create、skill_create、mcp 等）。
func (m *Manager) SetExtraToolFactory(f ExtraToolFactory) { m.extra = f }

// SetEnsureSkillDir 注入含包技能落盘回调（biz 提供），供 SkillBackend.Get 接通 BaseDirectory。
func (m *Manager) SetEnsureSkillDir(f EnsureSkillDirFunc) { m.ensureSkillDir = f }

// SetMaxStep 设置 ReAct 最大迭代步数。
func (m *Manager) SetMaxStep(n int) {
	if n > 0 {
		m.maxStep = n
	}
}

// Invalidate 让某个 agent 的缓存失效（配置更新后调用）。
func (m *Manager) Invalidate(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cache, key)
}

// Get 返回该 agent key 的可运行 Agent，必要时重建。
func (m *Manager) Get(ctx context.Context, key string) (adk.TypedAgent[*schema.AgenticMessage], error) {
	cfg, err := m.agents.GetByKey(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("agent %q 不存在: %w", key, err)
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("agent %q 已停用", key)
	}
	ver := cfg.UpdatedAt.UnixNano()
	m.mu.RLock()
	if c, ok := m.cache[key]; ok && c.updatedAt == ver {
		ag := c.agent
		m.mu.RUnlock()
		return ag, nil
	}
	m.mu.RUnlock()

	ag, err := m.build(ctx, cfg, 0)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.cache[key] = &cachedAgent{updatedAt: ver, agent: ag}
	m.mu.Unlock()
	return ag, nil
}

// maxSubAgentDepth 限制 subAgent 嵌套构建深度，防环。
const maxSubAgentDepth = 2

func (m *Manager) build(ctx context.Context, cfg *po.Agent, depth int) (adk.TypedAgent[*schema.AgenticMessage], error) {
	provider, model, err := m.llm.ResolveForAgent(ctx, cfg.LLMModelID)
	if err != nil {
		return nil, fmt.Errorf("解析 LLM 失败: %w", err)
	}
	cm, err := m.factory.Build(provider, model)
	if err != nil {
		return nil, err
	}

	// 工具集：内置工具 + subAgent(AgentTool) + 平台扩展工具
	tools, err := m.buildTools(ctx, cfg, depth)
	if err != nil {
		return nil, err
	}

	b := aggoagent.NewAgentBuilder(cm).
		WithName(cfg.Key).
		WithDescription(nonEmpty(cfg.Name, cfg.Key)).
		WithInstruction(cfg.Instruction).
		WithTools(tools...).
		WithMaxStep(m.maxStep)

	// SKILL 中间件：仅当绑定了 skill 时挂
	if ids := parseIntIDs(cfg.SkillIDs); len(ids) > 0 {
		backend := NewSkillBackend(m.skills, ids)
		backend.SetEnsureDir(m.ensureSkillDir)
		mw, err := skill.NewTyped[*schema.AgenticMessage](ctx, &skill.TypedConfig[*schema.AgenticMessage]{
			Backend: backend,
		})
		if err != nil {
			return nil, fmt.Errorf("构建 skill 中间件失败: %w", err)
		}
		b.WithMiddlewares(mw)
	}

	return b.Build(ctx)
}

func (m *Manager) buildTools(ctx context.Context, cfg *po.Agent, depth int) ([]tool.BaseTool, error) {
	var tools []tool.BaseTool

	// 内置工具（按 BuiltinTools 配置过滤；会话目录在运行时经 sessionID 注入，
	// 这里用 agent key 作为隔离命名空间，会话级隔离由 runner 传入）。
	builtinNames := parseStrList(cfg.BuiltinTools)
	bts, err := m.builtin.Tools(cfg.Key, builtinNames)
	if err != nil {
		return nil, err
	}
	tools = append(tools, bts...)

	// subAgent → AgentTool（默认上下文隔离）
	if depth < maxSubAgentDepth {
		subs, err := m.agents.ListSubAgents(ctx, cfg.ID)
		if err == nil {
			for _, sa := range subs {
				child, err := m.agents.GetByID(ctx, sa.ChildID)
				if err != nil || !child.Enabled {
					continue
				}
				childAgent, err := m.build(ctx, child, depth+1)
				if err != nil {
					continue
				}
				tools = append(tools, adk.NewTypedAgentTool(ctx, childAgent))
			}
		}
	}

	// 平台扩展工具（M6 注入）
	if m.extra != nil {
		ets, err := m.extra(ctx, cfg.Key, cfg)
		if err == nil {
			tools = append(tools, ets...)
		}
	}
	return tools, nil
}

func parseIntIDs(j datatypes.JSON) []int64 {
	var ids []int64
	if len(j) > 0 {
		_ = json.Unmarshal(j, &ids)
	}
	return ids
}

func parseStrList(j datatypes.JSON) []string {
	var s []string
	if len(j) > 0 {
		_ = json.Unmarshal(j, &s)
	}
	return s
}

func nonEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
