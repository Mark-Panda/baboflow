package biz

import (
	"github.com/google/wire"

	"baboflow/internal/biz/rulegokit"
	// 空导入：触发 nodes 包内各自定义节点（agent/archeryQuery/archerySchema…）的
	// init()，把它们注册进 rulego.Registry。biz 是被入口装配必定导入的包，
	// 借此保证无论 main 如何组装，自定义节点都已注册（否则含这些节点的链
	// Validate/RestorePublished 会因 NewNode 找不到类型而失败）。
	_ "baboflow/internal/biz/rulegokit/nodes"
)

// ProviderSet biz 层依赖。
var ProviderSet = wire.NewSet(
	rulegokit.NewManager,
	NewAuthUsecase,
	NewLLMUsecase,
	NewArcheryUsecase,
	NewComponentSync,
	NewRuleChainUsecase,
	NewAgentUsecase,
	NewMcpUsecase,
	NewBoardUsecase,
	NewAuditUsecase,
	NewCronUsecase,
	NewSkillUsecase,
	NewPlatformDeps,
	NewPlatformTools,
	// ChainRunner 由 *RuleChainUsecase 实现（RunPublished/RunPublishedAs）。
	wire.Bind(new(ChainRunner), new(*RuleChainUsecase)),
)
