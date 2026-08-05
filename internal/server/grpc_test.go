package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	reflectionv1alpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"gorm.io/gorm"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz"
	"baboflow/internal/conf"
	"baboflow/internal/service"
)

func TestServersRegisterProtoServicesAndKeepSidecars(t *testing.T) {
	authProto, archeryProto, llmProto, componentProto, ruleChainProto, agentProto, skillProto, mcpProto, boardProto, auditProto, cronProto := testProtoServices()
	rateLimiters := service.NewRateLimiters()
	t.Cleanup(rateLimiters.Stop)
	httpServer := NewHTTPServer(
		&conf.Config{HTTPAddr: ":0"},
		nil,
		authProto, archeryProto, llmProto, componentProto, ruleChainProto, agentProto, skillProto, mcpProto, boardProto, auditProto, cronProto,
		rateLimiters,
		service.NewFeishuHandler(nil), service.NewAgentHandler(nil), service.NewSkillHandler(nil),
		service.NewWsHub(nil, nil), biz.NewMcpUsecase(nil, nil), &gorm.DB{},
	)
	grpcServer := NewGRPCServer(
		&conf.Config{GRPCAddr: ":0"},
		nil,
		authProto, archeryProto, llmProto, componentProto, ruleChainProto, agentProto, skillProto, mcpProto, boardProto, auditProto, cronProto,
		rateLimiters,
	)

	assertHTTPRoutes(t, httpServer, []string{
		"/api/v1/auth/login",
		"/api/v1/archery/connections",
		"/api/v1/llm/providers",
		"/api/v1/components",
		"/api/v1/rule-chains",
		"/api/v1/agents",
		"/api/v1/skills",
		"/api/v1/mcp/servers",
		"/api/v1/boards",
		"/api/v1/audit-logs",
		"/api/v1/cron-jobs",
	})
	response := httptest.NewRecorder()
	httpServer.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, test := range []struct {
		method string
		path   string
		name   string
	}{
		{http.MethodGet, "/ws", "WebSocket"},
		{http.MethodGet, "/mcp/sse", "MCP SSE"},
		{http.MethodPost, "/api/v1/agent-assets", "agent asset bypass"},
		{http.MethodPost, "/api/v1/skills/package", "skill package bypass"},
		{http.MethodGet, "/api/v1/agents", "generated proto route"},
	} {
		response := httptest.NewRecorder()
		httpServer.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want %d", test.name, response.Code, http.StatusUnauthorized)
		}
	}

	assertGRPCReflectionServices(t, grpcServer, []string{
		"baboflow.v1.AuthService",
		"baboflow.v1.ArcheryService",
		"baboflow.v1.LLMService",
		"baboflow.v1.ComponentService",
		"baboflow.v1.RuleChainService",
		"baboflow.v1.AgentService",
		"baboflow.v1.SkillService",
		"baboflow.v1.McpService",
		"baboflow.v1.BoardService",
		"baboflow.v1.AuditService",
		"baboflow.v1.CronService",
	})
}

func TestIsTriggerOperation(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		want      bool
	}{
		{name: "MCP server test", operation: v1.OperationMcpServiceTestServer, want: true},
		{name: "component sync", operation: v1.OperationComponentServiceSync, want: true},
		{name: "MCP server list", operation: v1.OperationMcpServiceListServers, want: false},
		{name: "component list", operation: v1.OperationComponentServiceList, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTriggerOperation(context.Background(), tt.operation); got != tt.want {
				t.Fatalf("isTriggerOperation(%q) = %t, want %t", tt.operation, got, tt.want)
			}
		})
	}
}

func TestWriteSidecarNotFoundUsesNativeErrorBody(t *testing.T) {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	writeSidecarNotFound(ctx)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["message"] != "not found" || body["error"] != "not found" {
		t.Fatalf("unexpected error body: %s", response.Body.String())
	}
	if _, ok := body["code"]; ok {
		t.Fatalf("legacy code field must be absent: %s", response.Body.String())
	}
}

func assertGRPCReflectionServices(t *testing.T, server interface {
	Start(context.Context) error
	Stop(context.Context) error
	Endpoint() (*url.URL, error)
}, want []string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })

	url, err := server.Endpoint()
	if err != nil {
		t.Fatal(err)
	}
	if url == nil || url.Host == "" {
		t.Fatal("gRPC server did not start")
	}
	go func() { _ = server.Start(ctx) }()
	time.Sleep(time.Millisecond)
	_, port, err := net.SplitHostPort(url.Host)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient(net.JoinHostPort("127.0.0.1", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	stream, err := reflectionv1alpha.NewServerReflectionClient(conn).ServerReflectionInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&reflectionv1alpha.ServerReflectionRequest{
		MessageRequest: &reflectionv1alpha.ServerReflectionRequest_ListServices{},
	}); err != nil {
		t.Fatal(err)
	}
	response, err := stream.Recv()
	if err != nil {
		t.Fatalf("reflection request failed: %v", err)
	}
	services := map[string]bool{}
	for _, service := range response.GetListServicesResponse().Service {
		services[service.Name] = true
	}
	for _, name := range want {
		if !services[name] {
			t.Errorf("gRPC reflection did not report service %s", name)
		}
	}
}

func assertHTTPRoutes(t *testing.T, server *khttp.Server, want []string) {
	t.Helper()
	routes := map[string]bool{}
	if err := server.WalkRoute(func(route khttp.RouteInfo) error {
		routes[route.Path] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range want {
		if !routes[path] {
			t.Errorf("HTTP route %s is not registered", path)
		}
	}
}

func testProtoServices() (
	*service.AuthProtoService,
	*service.ArcheryProtoService,
	*service.LLMProtoService,
	*service.ComponentProtoService,
	*service.RuleChainProtoService,
	*service.AgentProtoService,
	*service.SkillProtoService,
	*service.McpProtoService,
	*service.BoardProtoService,
	*service.AuditProtoService,
	*service.CronProtoService,
) {
	return service.NewAuthProtoService(nil, nil),
		service.NewArcheryProtoService(nil, nil),
		service.NewLLMProtoService(nil, nil),
		service.NewComponentProtoService(nil, nil),
		service.NewRuleChainProtoService(nil, nil),
		service.NewAgentProtoService(nil),
		service.NewSkillProtoService(nil, nil),
		service.NewMcpProtoService(nil, nil, nil),
		service.NewBoardProtoService(nil, nil),
		service.NewAuditProtoService(nil),
		service.NewCronProtoService(nil)
}

var _ v1.AuthServiceServer = (*service.AuthProtoService)(nil)
