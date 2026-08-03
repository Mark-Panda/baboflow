package main

import (
	"context"
	"fmt"
	"os"
	"time"

	kratos "github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/joho/godotenv"

	"baboflow/internal/biz"
	"baboflow/internal/biz/agentkit"
	"baboflow/internal/biz/rulegokit"
	"baboflow/internal/biz/rulegokit/nodes"
	"baboflow/internal/conf"
	"baboflow/internal/data/po"
	"baboflow/internal/service"

	"github.com/cloudwego/eino/components/tool"
)

// App 聚合启动期需要触碰的依赖（装配运行期适配器、启动恢复、优雅停机）。
// HTTP server 本身由 wire 经 server.ProviderSet 构造完成。
type App struct {
	HTTPServer *khttp.Server

	Cfg          *conf.Config
	ChainUC      *biz.RuleChainUsecase
	McpUC        *biz.McpUsecase
	CronUC       *biz.CronUsecase
	BoardUC      *biz.BoardUsecase
	SkillUC      *biz.SkillUsecase
	ArcheryUC    *biz.ArcheryUsecase
	AuditUC      *biz.AuditUsecase
	AgentManager *agentkit.Manager
	Eng          *rulegokit.Manager
	CompSync     *biz.ComponentSync
	CompRepo     biz.ComponentRepo
	PlatformTools *biz.PlatformTools
	Tracer       *agentkit.Tracer

	// 各 handler：仅用于注入审计器。
	AuthH    *service.AuthHandler
	FeishuH  *service.FeishuHandler
	LLMH     *service.LLMHandler
	ArcheryH *service.ArcheryHandler
	ChainH   *service.RuleChainHandler
	SkillH   *service.SkillHandler
	McpH     *service.McpHandler
	BoardH   *service.BoardHandler
}

func newApp(
	httpSrv *khttp.Server,
	c *conf.Config,
	chainUC *biz.RuleChainUsecase,
	mcpUC *biz.McpUsecase,
	cronUC *biz.CronUsecase,
	boardUC *biz.BoardUsecase,
	skillUC *biz.SkillUsecase,
	archeryUC *biz.ArcheryUsecase,
	auditUC *biz.AuditUsecase,
	agentManager *agentkit.Manager,
	eng *rulegokit.Manager,
	compSync *biz.ComponentSync,
	compRepo biz.ComponentRepo,
	platformTools *biz.PlatformTools,
	tracer *agentkit.Tracer,
	authH *service.AuthHandler,
	feishuH *service.FeishuHandler,
	llmH *service.LLMHandler,
	archeryH *service.ArcheryHandler,
	chainH *service.RuleChainHandler,
	skillH *service.SkillHandler,
	mcpH *service.McpHandler,
	boardH *service.BoardHandler,
) *App {
	return &App{
		HTTPServer: httpSrv, Cfg: c,
		ChainUC: chainUC, McpUC: mcpUC, CronUC: cronUC, BoardUC: boardUC,
		SkillUC: skillUC, ArcheryUC: archeryUC, AuditUC: auditUC,
		AgentManager: agentManager, Eng: eng, CompSync: compSync, CompRepo: compRepo,
		PlatformTools: platformTools, Tracer: tracer,
		AuthH: authH, LLMH: llmH, ArcheryH: archeryH, ChainH: chainH,
		FeishuH: feishuH,
		SkillH: skillH, McpH: mcpH, BoardH: boardH,
	}
}

func main() {
	// .env 注入环境变量（不存在则忽略，纯环境变量部署亦可）。
	_ = godotenv.Load()
	cfg := conf.Load()

	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.name", "baboflow",
		"service.version", "0.1.0",
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)
	helper := log.NewHelper(logger)

	app, cleanup, err := wireApp(cfg)
	if err != nil {
		helper.Fatalf("装配依赖失败: %v", err)
	}
	defer cleanup()

	ctx := context.Background()
	injectRuntime(app, helper)
	boot(ctx, app, helper)

	ka := kratos.New(
		kratos.Name("baboflow"),
		kratos.Version("0.1.0"),
		kratos.Logger(logger),
		kratos.Server(app.HTTPServer),
		kratos.AfterStop(func(context.Context) error {
			app.CronUC.Stop()
			app.BoardUC.Stop()
			app.Eng.StopAll()
			return nil
		}),
	)
	helper.Infof("baboflow 启动 HTTP %s", cfg.HTTPAddr)
	if err := ka.Run(); err != nil {
		helper.Fatalf("服务退出: %v", err)
	}
}

// injectRuntime 装配 wire 无法表达的运行期注入：节点执行器、Agent/技能 runner、
// 平台工具工厂、组件变更回调、各 handler 审计器。必须在处理任何请求前完成。
func injectRuntime(app *App, helper *log.Helper) {
	// 以一段文本运行指定 Agent，返回最终文本。供 agent 规则链节点 / cron agent 任务 /
	// SKILL 生成复用。sessionID 以 "bg-" 前缀标识后台触发。
	runAgent := func(ctx context.Context, agentKey, prompt string) (string, error) {
		ag, err := app.AgentManager.Get(ctx, agentKey)
		if err != nil {
			return "", fmt.Errorf("获取 Agent %s 失败: %w", agentKey, err)
		}
		res, err := agentkit.Run(ctx, ag, nil, &agentkit.Input{Text: prompt}, nil, app.Tracer, "", "bg-"+agentKey)
		if err != nil {
			return "", err
		}
		return res.Text, nil
	}

	// 规则链节点执行器
	nodes.SetAgentRunner(runAgent)
	nodes.SetArcheryClientFactory(app.ArcheryUC.NewClientForInstance)

	// cron agent 任务（仅需 error）
	app.CronUC.SetAgentRunner(func(ctx context.Context, agentKey, prompt string) error {
		_, err := runAgent(ctx, agentKey, prompt)
		return err
	})

	// SKILL 生成器：把规则链信息喂给内置 agent-skill-generator，产出 SKILL.md。
	app.SkillUC.SetGenRunner(func(ctx context.Context, chainID, chainName, chainDesc string, inputSchema, dsl []byte) (string, error) {
		prompt := fmt.Sprintf(
			"请为以下已发布规则链生成标准 SKILL.md 并调用 skill_create 保存。\n规则链ID: %s\n名称: %s\n描述: %s\n输入 schema: %s\n规则链 DSL:\n%s",
			chainID, chainName, chainDesc, string(inputSchema), string(dsl),
		)
		return runAgent(ctx, "agent-skill-generator", prompt)
	})

	// 平台工具（检索组件 / 校验·查询·创建规则链 / 创建 SKILL）注入 Agent。
	app.AgentManager.SetExtraToolFactory(func(ctx context.Context, sessionID string, a *po.Agent) ([]tool.BaseTool, error) {
		return app.PlatformTools.Tools()
	})

	// 组件变更 → 自动同步对应 SKILL。
	app.CompSync.SetOnComponentChange(app.SkillUC.SyncComponentSkill)

	// 审计器（均 nil 守卫，可选）。
	app.AuthH.SetAuditor(app.AuditUC)
	app.FeishuH.SetAuditor(app.AuditUC)
	app.LLMH.SetAuditor(app.AuditUC)
	app.ArcheryH.SetAuditor(app.AuditUC)
	app.ChainH.SetAuditor(app.AuditUC)
	app.SkillH.SetAuditor(app.AuditUC)
	app.McpH.SetAuditor(app.AuditUC)
	app.BoardH.SetAuditor(app.AuditUC)
}

// boot 启动期恢复与后台任务：载入已发布规则链、恢复 MCP 暴露、启动 cron、周期组件同步。
// 失败仅记录不中断（除个别按需），保证进程可起。
func boot(ctx context.Context, app *App, helper *log.Helper) {
	if err := app.ChainUC.RestorePublished(ctx); err != nil {
		helper.Errorf("恢复已发布规则链失败: %v", err)
	}
	if err := app.McpUC.RestoreExposures(ctx); err != nil {
		helper.Errorf("恢复 MCP 暴露失败: %v", err)
	}
	app.CronUC.Start(ctx)

	// 周期同步组件注册表并回填组件 SKILL。
	biz.StartPeriodicSync(ctx, app.CompSync, 5*time.Minute, helper.Logger(), func(ctx context.Context) error {
		comps, err := app.CompRepo.ListAll(ctx)
		if err != nil {
			return err
		}
		return app.SkillUC.BackfillComponentSkills(ctx, comps)
	})
}
