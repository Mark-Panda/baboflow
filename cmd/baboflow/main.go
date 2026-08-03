package main

import (
	"context"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/joho/godotenv"

	"baboflow/internal/biz"
	"baboflow/internal/conf"
)

var (
	Name    = "baboflow"
	Version = "0.1.0"
)

func newApp(logger log.Logger, hs *http.Server, compSync *biz.ComponentSync, chainUC *biz.RuleChainUsecase, mcpUC *biz.McpUsecase, skillUC *biz.SkillUsecase, compRepo biz.ComponentRepo, cronUC *biz.CronUsecase) (*kratos.App, context.CancelFunc) {
	// 应用级可取消上下文：驱动周期同步等常驻 goroutine 的优雅退出。
	ctx, cancel := context.WithCancel(context.Background())
	// 组件自动同步：启动全量 + 周期对账（10 min）
	biz.StartPeriodicSync(ctx, compSync, 10*time.Minute, logger, func(ctx context.Context) error {
		comps, err := compRepo.ListAll(ctx)
		if err != nil {
			log.NewHelper(logger).Errorf("list components for skill backfill failed: %v", err)
			return err
		}
		return skillUC.BackfillComponentSkills(ctx, comps)
	})
	// 启动恢复所有已发布规则链到引擎池
	go func() {
		if err := chainUC.RestorePublished(context.Background()); err != nil {
			log.NewHelper(logger).Errorf("restore published chains failed: %v", err)
		}
	}()
	// 启动恢复所有已暴露的 MCP 工具
	go func() {
		if err := mcpUC.RestoreExposures(context.Background()); err != nil {
			log.NewHelper(logger).Errorf("restore mcp exposures failed: %v", err)
		}
	}()
	// 启动 Cron 调度器并加载启用中的任务
	go cronUC.Start(context.Background())
	return kratos.New(
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(hs),
	), cancel
}

func main() {
	// 加载 .env 到环境变量（不存在则忽略）
	_ = godotenv.Load()

	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.name", Name,
		"service.version", Version,
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)

	c := conf.Load()

	app, cleanup, err := initApp(c, logger)
	if err != nil {
		log.NewHelper(logger).Errorf("init app failed: %v", err)
		panic(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		log.NewHelper(logger).Errorf("run failed: %v", err)
		panic(err)
	}
}
