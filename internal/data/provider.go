package data

import (
	"github.com/google/wire"
)

// ProviderSet data 层依赖。
var ProviderSet = wire.NewSet(
	NewDB,
	NewAuthRepo,
	NewLLMRepo,
	NewComponentRepo,
	NewRuleChainRepo,
)
