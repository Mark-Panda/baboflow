//go:build wireinject
// +build wireinject

package main

import (
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	"baboflow/internal/biz"
	"baboflow/internal/conf"
	"baboflow/internal/data"
	"baboflow/internal/server"
	"baboflow/internal/service"
)

func initApp(*conf.Config, log.Logger) (*kratos.App, func(), error) {
	panic(wire.Build(
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
