package server

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-kratos/kratos/v2/middleware/selector"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"gorm.io/gorm"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz"
	"baboflow/internal/conf"
	"baboflow/internal/service"
)

// NewHTTPServer 注册 proto HTTP 服务，并保留非 proto 旁路处理器。
func NewHTTPServer(
	c *conf.Config,
	auth *biz.AuthUsecase,
	authProto *service.AuthProtoService,
	archeryProto *service.ArcheryProtoService,
	llmProto *service.LLMProtoService,
	componentProto *service.ComponentProtoService,
	ruleChainProto *service.RuleChainProtoService,
	agentProto *service.AgentProtoService,
	skillProto *service.SkillProtoService,
	mcpProto *service.McpProtoService,
	boardProto *service.BoardProtoService,
	auditProto *service.AuditProtoService,
	cronProto *service.CronProtoService,
	rateLimiters *service.RateLimiters,
	feishuH *service.FeishuHandler,
	agentH *service.AgentHandler,
	skillH *service.SkillHandler,
	wsHub *service.WsHub,
	mcpUC *biz.McpUsecase,
	db *gorm.DB,
) *khttp.Server {
	srv := khttp.NewServer(
		khttp.Address(c.HTTPAddr),
		khttp.Timeout(60*time.Second),
		khttp.Middleware(
			selector.Server(service.AuthMiddleware(auth)).Match(func(_ context.Context, operation string) bool {
				return operation != v1.OperationAuthServiceLogin
			}).Build(),
			selector.Server(rateLimiters.LoginMiddleware()).Match(func(_ context.Context, operation string) bool {
				return operation == v1.OperationAuthServiceLogin
			}).Build(),
			selector.Server(rateLimiters.TriggerMiddleware()).Match(isTriggerOperation).Build(),
		),
	)

	v1.RegisterAuthServiceHTTPServer(srv, authProto)
	v1.RegisterArcheryServiceHTTPServer(srv, archeryProto)
	v1.RegisterLLMServiceHTTPServer(srv, llmProto)
	v1.RegisterComponentServiceHTTPServer(srv, componentProto)
	v1.RegisterRuleChainServiceHTTPServer(srv, ruleChainProto)
	v1.RegisterAgentServiceHTTPServer(srv, agentProto)
	v1.RegisterSkillServiceHTTPServer(srv, skillProto)
	v1.RegisterMcpServiceHTTPServer(srv, mcpProto)
	v1.RegisterBoardServiceHTTPServer(srv, boardProto)
	v1.RegisterAuditServiceHTTPServer(srv, auditProto)
	v1.RegisterCronServiceHTTPServer(srv, cronProto)

	// Gin 仅承载不能由 protobuf HTTP 绑定表达的旁路：
	// health/metrics、WebSocket、MCP SSE、SPA、Feishu OAuth、multipart 上传与 raw 下载。
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	// OTel tracing：为每个请求生成 server span 并注入 request context，
	// 使日志 valuer trace.id/span.id 可取到值（当前无 exporter，span 不上报）。
	r.Use(otelgin.Middleware("baboflow"))

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
				writeSidecarNotFound(ctx)
				return
			}
			ctx.File(indexHTML)
		})
	}

	// 飞书 OAuth 登录公开；其余以下旁路必须显式保持 Gin Session 认证。
	r.GET("/api/v1/auth/feishu/login", feishuH.Login)
	r.GET("/api/v1/auth/feishu/callback", feishuH.Callback)
	sidecarAuth := service.GinAuthMiddleware(auth)
	r.POST("/api/v1/agent-assets", sidecarAuth, agentH.UploadAsset)
	r.GET("/api/v1/agent-assets/:assetId", sidecarAuth, agentH.GetAsset)
	r.POST("/api/v1/skills/package", sidecarAuth, skillH.UploadPackage)
	r.GET("/api/v1/skills/:id/package", sidecarAuth, skillH.DownloadPackage)

	srv.HandlePrefix("/", r)
	return srv
}

func writeSidecarNotFound(ctx *gin.Context) {
	ctx.JSON(http.StatusNotFound, gin.H{"message": "not found", "error": "not found"})
}

func isTriggerOperation(_ context.Context, operation string) bool {
	switch operation {
	case v1.OperationLLMServiceTestProvider,
		v1.OperationLLMServiceTestModel,
		v1.OperationArcheryServiceTestConnection,
		v1.OperationArcheryServiceSyncInstances,
		v1.OperationMcpServiceTestServer,
		v1.OperationComponentServiceSync,
		v1.OperationRuleChainServiceRun,
		v1.OperationRuleChainServiceDebug,
		v1.OperationBoardServiceTriggerTask:
		return true
	default:
		return false
	}
}

// Shutdown 占位（如需优雅关闭 db 等）。
func Shutdown(ctx context.Context) error { return nil }
