package project

import (
	"go.uber.org/fx"
)

// Module provides the project service dependencies to the fx container
var Module = fx.Options(
	fx.Provide(New),
)
