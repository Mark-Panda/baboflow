package service

import (
	"github.com/google/wire"
)

// ProviderSet service 层依赖。
var ProviderSet = wire.NewSet(
	NewAuthHandler,
	NewLLMHandler,
	NewArcheryHandler,
	NewComponentHandler,
	NewRuleChainHandler,
	NewAgentHandler,
	NewSkillHandler,
	NewMcpHandler,
	NewBoardHandler,
	NewAuditHandler,
	NewCronHandler,
	NewWsHub,
)
