package data

import (
	"github.com/google/wire"
	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/biz/agentkit"
	"baboflow/internal/conf"
)

// ProviderSet data 层依赖。
var ProviderSet = wire.NewSet(
	NewDB,
	NewAuthRepo,
	NewLLMRepo,
	NewArcheryRepo,
	NewComponentRepo,
	NewRuleChainRepo,
	NewAuditRepo,
	NewBoardRepo,
	NewCronRepo,
	NewMcpRepo,
	NewAgentRepo,
	NewAgentDataRepo,
	NewAgentkitSkillRepo,
	NewBizSkillDataRepo,
	NewAgentkitLLMResolver,
	NewAssetStore,
)

// ---- 接口包装：wire 不能跨包引用未导出返回类型（*skillRepo / *llmResolver），
// 也不能直接把标量参数（c.Workspace）传给 NewLocalAssetStore，这里提供导出 provider。 ----

// NewAgentkitSkillRepo 供 agentkit.Manager 使用。
func NewAgentkitSkillRepo(db *gorm.DB) agentkit.SkillRepo { return NewSkillRepo(db) }

// NewBizSkillDataRepo 供 biz.SkillUsecase 使用（与上面是同一 *skillRepo 的两个接口视图）。
func NewBizSkillDataRepo(db *gorm.DB) biz.SkillDataRepo { return NewSkillRepo(db) }

// NewAgentkitLLMResolver 供 agentkit.Manager 解析 LLM 模型。
func NewAgentkitLLMResolver(db *gorm.DB) agentkit.LLMResolver { return NewLLMResolver(db) }

// NewAssetStore Agent 产物本地存储（沙箱目录来自配置）。
func NewAssetStore(c *conf.Config) biz.AssetStore { return NewLocalAssetStore(c.Workspace) }
