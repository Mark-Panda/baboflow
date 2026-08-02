package biz

import (
	"github.com/google/wire"

	"baboflow/internal/biz/rulegokit"
)

// ProviderSet biz 层依赖。
var ProviderSet = wire.NewSet(
	rulegokit.NewManager,
	NewAuthUsecase,
	NewLLMUsecase,
	NewComponentSync,
	NewRuleChainUsecase,
)
