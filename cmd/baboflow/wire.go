//go:build wireinject
// +build wireinject

package main

import (
	"github.com/google/wire"

	"baboflow/internal/biz"
	"baboflow/internal/biz/agentkit"
	"baboflow/internal/conf"
	"baboflow/internal/data"
	"baboflow/internal/server"
	"baboflow/internal/service"
)

// wireApp 由 wire 生成具体实现（见 wire_gen.go）。
func wireApp(c *conf.Config) (*App, func(), error) {
	panic(wire.Build(
		data.ProviderSet,
		agentkit.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		server.ProviderSet,
		newApp,
	))
}
