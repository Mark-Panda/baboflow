package server

import (
	"context"

	"github.com/go-kratos/kratos/v2/middleware/selector"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz"
	"baboflow/internal/conf"
	"baboflow/internal/service"
)

// NewGRPCServer 注册全部 proto gRPC 服务，并在传输边界执行认证和限流。
func NewGRPCServer(
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
) *kgrpc.Server {
	srv := kgrpc.NewServer(
		kgrpc.Address(c.GRPCAddr),
		kgrpc.Middleware(
			selector.Server(service.AuthMiddleware(auth)).Match(func(_ context.Context, operation string) bool {
				return operation != v1.OperationAuthServiceLogin
			}).Build(),
			selector.Server(rateLimiters.LoginMiddleware()).Match(func(_ context.Context, operation string) bool {
				return operation == v1.OperationAuthServiceLogin
			}).Build(),
			selector.Server(rateLimiters.TriggerMiddleware()).Match(isTriggerOperation).Build(),
		),
	)
	// Kratos 默认注册 gRPC reflection；不要传 DisableReflection，供受控客户端发现服务。

	v1.RegisterAuthServiceServer(srv, authProto)
	v1.RegisterArcheryServiceServer(srv, archeryProto)
	v1.RegisterLLMServiceServer(srv, llmProto)
	v1.RegisterComponentServiceServer(srv, componentProto)
	v1.RegisterRuleChainServiceServer(srv, ruleChainProto)
	v1.RegisterAgentServiceServer(srv, agentProto)
	v1.RegisterSkillServiceServer(srv, skillProto)
	v1.RegisterMcpServiceServer(srv, mcpProto)
	v1.RegisterBoardServiceServer(srv, boardProto)
	v1.RegisterAuditServiceServer(srv, auditProto)
	v1.RegisterCronServiceServer(srv, cronProto)
	return srv
}
