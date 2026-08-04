package agentkit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	aggoagent "github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"baboflow/internal/conf"
	"baboflow/internal/data/po"
	"baboflow/internal/memorystore"
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
	ensureSkillDir  EnsureSkillDirFunc
	maxStep         int
	mu              sync.RWMutex
	cache           map[string]*cachedAgent
	db              *gorm.DB
	cfg             *conf.Config
	memoryProviders map[string]memory.MemoryProvider
	memoryRefs      map[string]int
	retiredMemory   []memory.MemoryProvider
}

// ExtraToolFactory 让上层（M6）注入 rulechain/skill/mcp 等平台工具。
// 返回的工具会追加到 agent 工具表。sessionID 用于工作区/权限上下文。
type ExtraToolFactory func(ctx context.Context, sessionID string, a *po.Agent) ([]tool.BaseTool, error)

type cachedAgent struct {
	updatedAt int64
	agent     adk.TypedAgent[*schema.AgenticMessage]
	memoryKey string
}

type fallbackMemoryProvider struct {
	memory.MemoryProvider
}

func (p *fallbackMemoryProvider) Retrieve(ctx context.Context, req *memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	result, err := p.MemoryProvider.Retrieve(ctx, req)
	useMemoryHistory := true
	if raw, ok := adk.GetSessionValue(ctx, "useMemoryHistory"); ok {
		if value, ok := raw.(bool); ok {
			useMemoryHistory = value
		}
	}
	if err != nil || result == nil {
		result = &memory.RetrieveResult{}
		if !useMemoryHistory {
			if history, ok := adk.GetSessionValue(ctx, "businessHistory"); ok {
				if messages, ok := history.([]*schema.AgenticMessage); ok {
					result.HistoryMessages = messages
				}
			}
		}
	}
	return memoryResultForRun(result, useMemoryHistory), nil
}

func memoryResultForRun(result *memory.RetrieveResult, useMemoryHistory bool) *memory.RetrieveResult {
	if result == nil {
		return &memory.RetrieveResult{}
	}
	if useMemoryHistory {
		result.HistoryMessages = nil
	}
	return result
}

func NewManager(
	agents AgentRepo,
	skills SkillRepo,
	llm LLMResolver,
	factory *ModelFactory,
	builtin *BuiltinTools,
) *Manager {
	return &Manager{
		agents:          agents,
		skills:          skills,
		llm:             llm,
		factory:         factory,
		builtin:         builtin,
		maxStep:         16,
		cache:           map[string]*cachedAgent{},
		memoryProviders: map[string]memory.MemoryProvider{},
		memoryRefs:      map[string]int{},
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

// SetMemoryDB 注入记忆模块使用的共享数据库连接和配置。
// 测试或不需要持久化记忆的场景可以不调用，此时 Agent 行为保持不变。
func (m *Manager) SetMemoryDB(db *gorm.DB, cfg *conf.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db != nil && (m.db != db || m.cfg != cfg) {
		for key, cached := range m.cache {
			delete(m.cache, key)
			m.retireMemoryLocked(m.releaseMemoryLocked(cached))
		}
	}
	m.db = db
	m.cfg = cfg
}

// DeleteSessionData 清理指定会话在记忆存储中的消息和摘要。
func (m *Manager) DeleteSessionData(ctx context.Context, userID, sessionID string) error {
	m.mu.RLock()
	db := m.db
	m.mu.RUnlock()
	if db == nil {
		return nil
	}
	store, err := memorystore.NewPostgresStorage(db)
	if err != nil {
		return err
	}
	return store.DeleteSessionData(ctx, userID, sessionID)
}

// Invalidate 让某个 agent 的缓存失效（配置更新后调用）。
func (m *Manager) Invalidate(key string) {
	m.mu.Lock()
	cached := m.cache[key]
	delete(m.cache, key)
	m.retireMemoryLocked(m.releaseMemoryLocked(cached))
	m.mu.Unlock()
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

	ag, memoryKey, err := m.build(ctx, cfg, 0)
	if err != nil {
		return nil, err
	}
	m.cacheAgent(key, ver, ag, memoryKey)
	return ag, nil
}

func (m *Manager) cacheAgent(key string, ver int64, ag adk.TypedAgent[*schema.AgenticMessage], memoryKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.cache[key]; ok && current.updatedAt > ver {
		m.retireMemoryLocked(m.releaseMemoryKeyLocked(memoryKey))
		return
	}
	old := m.cache[key]
	m.cache[key] = &cachedAgent{updatedAt: ver, agent: ag, memoryKey: memoryKey}
	m.retireMemoryLocked(m.releaseMemoryLocked(old))
}

// maxSubAgentDepth 限制 subAgent 嵌套构建深度，防环。
const maxSubAgentDepth = 2

func (m *Manager) build(ctx context.Context, cfg *po.Agent, depth int) (adk.TypedAgent[*schema.AgenticMessage], string, error) {
	provider, model, err := m.llm.ResolveForAgent(ctx, cfg.LLMModelID)
	if err != nil {
		return nil, "", fmt.Errorf("解析 LLM 失败: %w", err)
	}
	cm, err := m.factory.Build(provider, model)
	if err != nil {
		return nil, "", err
	}

	// 工具集：内置工具 + subAgent(AgentTool) + 平台扩展工具
	tools, err := m.buildTools(ctx, cfg, depth)
	if err != nil {
		return nil, "", err
	}

	b := aggoagent.NewAgentBuilder(cm).
		WithName(cfg.Key).
		WithDescription(nonEmpty(cfg.Name, cfg.Key)).
		WithInstruction(cfg.Instruction).
		WithTools(tools...).
		WithMaxStep(m.maxStep)

	var memoryProvider memory.MemoryProvider
	memoryKey := ""
	if depth == 0 && m.memoryEnabled() {
		memoryKey = memoryProviderKey(provider, model)
		memoryProvider, err = m.acquireMemoryProvider(memoryKey, cm)
		if err != nil {
			return nil, "", fmt.Errorf("构建记忆 provider 失败: %w", err)
		}
		b.WithMemory(&fallbackMemoryProvider{MemoryProvider: memoryProvider})
	}

	// SKILL 中间件：仅当绑定了 skill 时挂
	if ids := parseIntIDs(cfg.SkillIDs); len(ids) > 0 {
		backend := NewSkillBackend(m.skills, ids)
		backend.SetEnsureDir(m.ensureSkillDir)
		mw, err := skill.NewTyped[*schema.AgenticMessage](ctx, &skill.TypedConfig[*schema.AgenticMessage]{
			Backend: backend,
		})
		if err != nil {
			if memoryProvider != nil {
				m.releaseMemoryProvider(memoryKey)
			}
			return nil, "", fmt.Errorf("构建 skill 中间件失败: %w", err)
		}
		b.WithMiddlewares(mw)
	}

	ag, err := b.Build(ctx)
	if err != nil {
		if memoryProvider != nil {
			m.releaseMemoryProvider(memoryKey)
		}
		return nil, "", err
	}
	return ag, memoryKey, nil
}

func (m *Manager) memoryEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.db != nil && m.cfg != nil && m.cfg.MemoryEnabled
}

func (m *Manager) newMemoryProvider(cm model.AgenticModel) (memory.MemoryProvider, error) {
	m.mu.RLock()
	db, cfg := m.db, m.cfg
	m.mu.RUnlock()
	store, err := memorystore.NewPostgresStorage(db)
	if err != nil {
		return nil, err
	}
	memoryCfg := builtin.DefaultMemoryConfig()
	memoryCfg.EnableSessionSummary = cfg.MemorySessionSummary
	memoryCfg.EnableEventSearch = cfg.MemoryEventSearch
	memoryCfg.MemoryLimit = cfg.MemoryLimit
	return memory.GlobalRegistry().CreateProvider("builtin", &builtin.ProviderConfig{
		ChatModel:    cm,
		Storage:      store,
		MemoryConfig: memoryCfg,
	})
}

func memoryProviderKey(provider *po.LLMProvider, model *po.LLMModel) string {
	return fmt.Sprintf("%d:%d:%d:%d", provider.ID, model.ID, provider.UpdatedAt.UnixNano(), model.UpdatedAt.UnixNano())
}

func (m *Manager) acquireMemoryProvider(key string, cm model.AgenticModel) (memory.MemoryProvider, error) {
	m.mu.Lock()
	if provider, ok := m.memoryProviders[key]; ok {
		m.memoryRefs[key]++
		m.mu.Unlock()
		return provider, nil
	}
	m.mu.Unlock()

	provider, err := m.newMemoryProvider(cm)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if existing, ok := m.memoryProviders[key]; ok {
		m.memoryRefs[key]++
		m.mu.Unlock()
		_ = provider.Close()
		return existing, nil
	}
	m.memoryProviders[key] = provider
	m.memoryRefs[key] = 1
	m.mu.Unlock()
	return provider, nil
}

func (m *Manager) releaseMemoryProvider(key string) {
	m.mu.Lock()
	provider := m.releaseMemoryKeyLocked(key)
	m.mu.Unlock()
	if provider != nil {
		_ = provider.Close()
	}
}

func (m *Manager) retireMemoryLocked(provider memory.MemoryProvider) {
	if provider != nil {
		m.retiredMemory = append(m.retiredMemory, provider)
	}
}

func (m *Manager) releaseMemoryLocked(cached *cachedAgent) memory.MemoryProvider {
	if cached == nil {
		return nil
	}
	return m.releaseMemoryKeyLocked(cached.memoryKey)
}

func (m *Manager) releaseMemoryKeyLocked(key string) memory.MemoryProvider {
	if key == "" {
		return nil
	}
	m.memoryRefs[key]--
	if m.memoryRefs[key] > 0 {
		return nil
	}
	provider := m.memoryProviders[key]
	delete(m.memoryRefs, key)
	delete(m.memoryProviders, key)
	return provider
}

// Close 停止记忆模块的异步 worker。
func (m *Manager) Close() error {
	m.mu.Lock()
	providers := make([]memory.MemoryProvider, 0, len(m.memoryProviders)+len(m.retiredMemory))
	for key, provider := range m.memoryProviders {
		providers = append(providers, provider)
		delete(m.memoryProviders, key)
		delete(m.memoryRefs, key)
	}
	providers = append(providers, m.retiredMemory...)
	m.retiredMemory = nil
	m.mu.Unlock()
	var firstErr error
	for _, provider := range providers {
		if err := provider.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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
				childAgent, _, err := m.build(ctx, child, depth+1)
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
