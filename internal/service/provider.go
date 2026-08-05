package service

import (
	"github.com/google/wire"
)

// ProviderSet service 层依赖。
var ProviderSet = wire.NewSet(
	NewRateLimiters,
	NewFeishuHandler,
	NewAgentHandler,
	NewSkillHandler,
	NewWsHub,
	NewAuthProtoService,
	NewArcheryProtoService,
	NewLLMProtoService,
	NewComponentProtoService,
	NewRuleChainProtoService,
	NewAgentProtoService,
	NewSkillProtoService,
	NewMcpProtoService,
	NewBoardProtoService,
	NewAuditProtoService,
	NewCronProtoService,
)
