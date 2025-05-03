package xds

import (
	"go.uber.org/fx"
)

func RunXDS(xds *Service) {
	// nothing needed here
}

// Module exports XDS service and related components
var Module = fx.Options(
	fx.Provide(New),
	fx.Invoke(RunXDS),
)
