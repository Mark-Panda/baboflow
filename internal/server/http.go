package server

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"baboflow/internal/biz"
	"baboflow/internal/conf"
	"baboflow/internal/service"
)

// NewHTTPServer 用 Gin 作为 Handler 挂进 Kratos http.Server，注册全部路由。
func NewHTTPServer(
	c *conf.Config,
	auth *biz.AuthUsecase,
	authH *service.AuthHandler,
	feishuH *service.FeishuHandler,
	llmH *service.LLMHandler,
	archeryH *service.ArcheryHandler,
	compH *service.ComponentHandler,
	chainH *service.RuleChainHandler,
	agentH *service.AgentHandler,
	skillH *service.SkillHandler,
	mcpH *service.McpHandler,
	boardH *service.BoardHandler,
	auditH *service.AuditHandler,
	cronH *service.CronHandler,
	wsHub *service.WsHub,
	mcpUC *biz.McpUsecase,
	db *gorm.DB,
) *khttp.Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Prometheus 指标
	biz.RegisterMetrics()

	// 健康检查
	r.GET("/healthz", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })
	// 就绪检查：db 可达
	r.GET("/readyz", func(ctx *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(ctx.Request.Context()) != nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{"ready": false, "db": "down"})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"ready": true, "db": "up"})
	})
	// Prometheus 指标端点
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// WebSocket 端点（内部自行 cookie 鉴权）
	r.GET("/ws", wsHub.Handle)

	// MCP SSE 端点（外部 MCP 客户端连入）。
	// mcp-go SSEServer 自带 mux 且按绝对路径匹配（basePath=/mcp → /mcp/sse、/mcp/message），
	// 必须以完整 URL 路径挂载，不能被 Gin 前缀截断。
	// 安全：该端点执行已发布规则链，必须鉴权（会话 Cookie 或 MCP_AUTH_TOKEN Bearer），
	// 否则任何可达 :8000 的主机都能未授权枚举/调用全部暴露的规则链。
	mcpAuth := service.MCPAuthMiddleware(auth, c.MCPAuthToken)
	sse := mcpUC.SSEHandler()
	r.Any("/mcp/sse", mcpAuth, gin.WrapH(sse))
	r.Any("/mcp/message", mcpAuth, gin.WrapH(sse))
	r.Any("/mcp", mcpAuth, gin.WrapH(sse))

	// 前端静态资源：仅当 web/dist 存在时注册（dev 用 vite 起前端时可为空）。
	const distDir = "./web/dist"
	const indexHTML = distDir + "/index.html"
	if info, err := os.Stat(indexHTML); err == nil && !info.IsDir() {
		r.Static("/assets", distDir+"/assets")
		// SPA 路由回退：非 /api、非 /ws 的 GET 一律回 index.html。
		r.NoRoute(func(ctx *gin.Context) {
			p := ctx.Request.URL.Path
			if ctx.Request.Method != http.MethodGet ||
				strings.HasPrefix(p, "/api/") || p == "/api" ||
				strings.HasPrefix(p, "/ws") {
				ctx.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
				return
			}
			ctx.File(indexHTML)
		})
	}

	api := r.Group("/api/v1")

	// 登录限流：IP 维度令牌桶（防爆破），每 2s 补 1 个、容量 5。
	loginLimiter := service.RateLimitMiddleware(func(c *gin.Context) string { return "login:" + c.ClientIP() }, rate.Every(2*time.Second), 5)
	// 运行/调试/测试触发限流：每用户 1s 补 1、容量 10。
	triggerLimiter := service.RateLimitMiddleware(func(c *gin.Context) string {
		return "trigger:" + c.ClientIP()
	}, rate.Every(1*time.Second), 10)

	// Auth（无需登录）
	authG := api.Group("/auth")
	{
		authG.POST("/login", loginLimiter, authH.Login)
		// 飞书 OAuth 登录（公开）：入口 302 到飞书授权页，回调发证后 302 回前端。
		authG.GET("/feishu/login", feishuH.Login)
		authG.GET("/feishu/callback", feishuH.Callback)
	}
	// 需登录的 auth
	authAuthed := api.Group("/auth", service.AuthMiddleware(auth))
	{
		authAuthed.POST("/logout", authH.Logout)
		authAuthed.GET("/me", authH.Me)
		authAuthed.PUT("/password", authH.ChangePassword)
	}

	// 以下全部需登录
	authed := api.Group("", service.AuthMiddleware(auth))
	{
		// LLM 配置
		llm := authed.Group("/llm")
		{
			llm.GET("/providers", llmH.ListProviders)
			llm.POST("/providers", llmH.CreateProvider)
			llm.PUT("/providers/:id", llmH.UpdateProvider)
			llm.DELETE("/providers/:id", llmH.DeleteProvider)
			llm.POST("/providers/:id/test", triggerLimiter, llmH.TestProvider)
			llm.GET("/providers/:id/models/remote", llmH.RemoteModels)
			llm.GET("/providers/:id/models", llmH.ListModels)
			llm.POST("/providers/:id/models", llmH.CreateModels)
			llm.PUT("/models/:modelId", llmH.UpdateModel)
			llm.DELETE("/models/:modelId", llmH.DeleteModel)
			llm.POST("/models/:modelId/default", llmH.SetDefaultModel)
			llm.POST("/models/:modelId/test", triggerLimiter, llmH.TestModel)
		}

		// Archery 连接
		archery := authed.Group("/archery")
		{
			archery.GET("/connections", archeryH.ListConnections)
			archery.POST("/connections", archeryH.CreateConnection)
			archery.GET("/connections/:id", archeryH.GetConnection)
			archery.PUT("/connections/:id", archeryH.UpdateConnection)
			archery.DELETE("/connections/:id", archeryH.DeleteConnection)
			archery.POST("/connections/:id/test", triggerLimiter, archeryH.TestConnection)
			archery.GET("/connections/:id/instances", archeryH.ListInstances)
			archery.POST("/connections/:id/sync-instances", triggerLimiter, archeryH.SyncInstances)
		}

		// 组件
		comp := authed.Group("/components")
		{
			comp.GET("", compH.List)
			comp.GET("/sync", compH.SyncStatus)
			comp.POST("/sync", compH.TriggerSync)
		}

		// 规则链
		chain := authed.Group("/chains")
		{
			chain.GET("", chainH.List)
			chain.POST("", chainH.Create)
			chain.POST("/import", chainH.Import)
			chain.POST("/validate", chainH.Validate)
			chain.GET("/runs", chainH.Runs)
			chain.GET("/runs/:runId", chainH.RunDetail)

			chain.GET("/:id", chainH.Get)
			chain.PUT("/:id", chainH.Update)
			chain.DELETE("/:id", chainH.Delete)
			chain.POST("/:id/publish", chainH.Publish)
			chain.POST("/:id/offline", chainH.Offline)
			chain.GET("/:id/versions", chainH.Versions)
			chain.POST("/:id/rollback", chainH.Rollback)
			chain.GET("/:id/export", chainH.Export)
			chain.POST("/:id/run", triggerLimiter, chainH.Run)
			chain.POST("/:id/debug", triggerLimiter, chainH.Debug)
			// M6：生成 SKILL / 暴露 MCP
			chain.POST("/:id/skill", skillH.Generate)
			chain.POST("/:id/expose", mcpH.Expose)
		}

		// Agent（M5）
		ag := authed.Group("/agents")
		{
			ag.GET("", agentH.List)
			ag.POST("", agentH.Create)
			ag.GET("/:key", agentH.Get)
			ag.PUT("/:key", agentH.Update)
			ag.DELETE("/:key", agentH.Delete)

			ag.GET("/:key/sessions", agentH.ListSessions)
			ag.POST("/:key/sessions", agentH.CreateSession)

			ag.GET("/sessions/:sessionId/messages", agentH.ListMessages)
			ag.DELETE("/sessions/:sessionId", agentH.DeleteSession)

			ag.POST("/assets", agentH.UploadAsset)
			ag.GET("/assets/:assetId", agentH.GetAsset)
		}

		// SKILL（M6a）
		skill := authed.Group("/skills")
		{
			skill.GET("", skillH.List)
			skill.POST("", skillH.Upload)
			skill.GET("/:id", skillH.Get)
			skill.DELETE("/:id", skillH.Delete)
		}

		// MCP（M6b）
		mcp := authed.Group("/mcp")
		{
			mcp.GET("/servers", mcpH.ListServers)
			mcp.POST("/servers", mcpH.CreateServer)
			mcp.PUT("/servers/:id", mcpH.UpdateServer)
			mcp.DELETE("/servers/:id", mcpH.DeleteServer)
			mcp.POST("/servers/:id/toggle", mcpH.ToggleServer)
			mcp.POST("/servers/:id/test", mcpH.TestServer)
			mcp.GET("/exposures", mcpH.ListExposures)
			mcp.DELETE("/exposures/:id", mcpH.RemoveExposure)
		}

		// 看板（M6d）
		board := authed.Group("/boards")
		{
			board.GET("", boardH.ListBoards)
			board.POST("", boardH.CreateBoard)
			board.GET("/:id", boardH.GetBoard)
			board.PUT("/:id", boardH.UpdateBoard)
			board.DELETE("/:id", boardH.DeleteBoard)

			board.POST("/:id/columns", boardH.CreateColumn)
			board.PUT("/columns/:cid", boardH.UpdateColumn)
			board.DELETE("/columns/:cid", boardH.DeleteColumn)

			board.POST("/columns/:cid/tasks", boardH.CreateTask)
		}
		// 任务（挂在 /tasks 下，避免与列路由冲突）
		task := authed.Group("/tasks")
		{
			task.PUT("/:id", boardH.UpdateTask)
			task.DELETE("/:id", boardH.DeleteTask)
			task.POST("/:id/move", boardH.MoveTask)
			task.POST("/:id/trigger", triggerLimiter, boardH.TriggerTask)
		}

		// 审计（M7a，仅 admin；当前单租户全为 admin）
		authed.GET("/audit", auditH.List)

		// Cron 定时任务（M7b）
		cron := authed.Group("/cron")
		{
			cron.GET("", cronH.List)
			cron.POST("", cronH.Create)
			cron.PUT("/:id", cronH.Update)
			cron.DELETE("/:id", cronH.Delete)
			cron.POST("/:id/toggle", cronH.Toggle)
		}
	}

	srv := khttp.NewServer(
		khttp.Address(c.HTTPAddr),
		khttp.Timeout(60*time.Second),
	)
	srv.HandlePrefix("/", r)
	return srv
}

// Shutdown 占位（如需优雅关闭 db 等）。
func Shutdown(ctx context.Context) error { return nil }
