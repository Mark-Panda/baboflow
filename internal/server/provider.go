package server

import (
	"github.com/google/wire"
)

// ProviderSet server 层依赖。
var ProviderSet = wire.NewSet(
	NewHTTPServer,
)
