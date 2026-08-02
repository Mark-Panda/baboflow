package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/datatypes"

	"baboflow/internal/data/po"
)

// McpDataRepo MCP server + exposure 持久化接口。
type McpDataRepo interface {
	// server
	ListServers(ctx context.Context) ([]po.McpServer, error)
	GetServer(ctx context.Context, id int64) (*po.McpServer, error)
	CreateServer(ctx context.Context, s *po.McpServer) error
	UpdateServer(ctx context.Context, s *po.McpServer) error
	DeleteServer(ctx context.Context, id int64) error

	// exposure
	ListExposures(ctx context.Context) ([]po.McpExposure, error)
	ListEnabledExposures(ctx context.Context) ([]po.McpExposure, error)
	GetExposure(ctx context.Context, id int64) (*po.McpExposure, error)
	GetExposureByTool(ctx context.Context, toolName string) (*po.McpExposure, error)
	CreateExposure(ctx context.Context, e *po.McpExposure) error
	UpdateExposure(ctx context.Context, e *po.McpExposure) error
	DeleteExposure(ctx context.Context, id int64) error
}

// ChainRunner 执行已发布链（由 RuleChainUsecase 提供）。
// data 为规则链 msg.data 的 JSON 字符串。
type ChainRunner interface {
	RunPublished(ctx context.Context, chainID string, data string) (string, error)
}

// TypedChainRunner 可选：带触发来源的执行（用于 chain_run 正确留痕 manual/task/mcp/cron）。
// RuleChainUsecase 实现了它；调用方按需断言。
type TypedChainRunner interface {
	RunPublishedAs(ctx context.Context, chainID string, data string, trigger string) (string, error)
}

// runChainWithTrigger 优先用带 trigger 的执行，否则退回默认。
func runChainWithTrigger(ctx context.Context, r ChainRunner, chainID, data, trigger string) (string, error) {
	if tr, ok := r.(TypedChainRunner); ok {
		return tr.RunPublishedAs(ctx, chainID, data, trigger)
	}
	return r.RunPublished(ctx, chainID, data)
}

// McpUsecase MCP 服务配置 + 规则链工具暴露。
type McpUsecase struct {
	repo       McpDataRepo
	runner     ChainRunner
	mcpServer  *server.MCPServer
	sseServer  *server.SSEServer
	mu         sync.Mutex
	loaded     map[string]bool // toolName → 已注册
}

func NewMcpUsecase(repo McpDataRepo, runner ChainRunner) *McpUsecase {
	ms := server.NewMCPServer("baboflow", "0.1.0",
		server.WithToolCapabilities(true),
	)
	// SSE server 自带 mux（绝对路径匹配），以 /mcp 为前缀，挂到主 HTTP 根上。
	// 单租户内部部署：关闭 localhost Host 白名单限制（如需再加可用 WithAllowedHosts）。
	sse := server.NewSSEServer(ms,
		server.WithStaticBasePath("/mcp"),
		server.WithSSEDisableLocalhostProtection(true),
	)
	return &McpUsecase{
		repo:      repo,
		runner:    runner,
		mcpServer: ms,
		sseServer: sse,
		loaded:    map[string]bool{},
	}
}

// SSEHandler 返回挂到主 HTTP 服务的 MCP SSE 端点（/mcp）。
func (uc *McpUsecase) SSEHandler() *server.SSEServer { return uc.sseServer }

// ---- Server 配置 CRUD ----

func (uc *McpUsecase) ListServers(ctx context.Context) ([]po.McpServer, error) {
	return uc.repo.ListServers(ctx)
}

// GetServerForTest 供连通性检测读取 server 配置。
func (uc *McpUsecase) GetServerForTest(ctx context.Context, id int64) (*po.McpServer, error) {
	s, err := uc.repo.GetServer(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return s, nil
}

type McpServerInput struct {
	Name      string         `json:"name" binding:"required"`
	Transport string         `json:"transport"`
	Endpoint  string         `json:"endpoint"`
	Command   string         `json:"command"`
	Args      []string       `json:"args"`
	Env       map[string]string `json:"env"`
}

func (uc *McpUsecase) CreateServer(ctx context.Context, in *McpServerInput) (*po.McpServer, error) {
	if in.Transport == "" {
		in.Transport = "sse"
	}
	args, _ := json.Marshal(in.Args)
	env, _ := json.Marshal(in.Env)
	s := &po.McpServer{
		Name: in.Name, Transport: in.Transport, Endpoint: in.Endpoint,
		Command: in.Command, Args: args, Env: env, Status: "disabled",
	}
	if err := uc.repo.CreateServer(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (uc *McpUsecase) UpdateServer(ctx context.Context, id int64, in *McpServerInput) error {
	s, err := uc.repo.GetServer(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	s.Name = in.Name
	s.Endpoint = in.Endpoint
	s.Command = in.Command
	if in.Transport != "" {
		s.Transport = in.Transport
	}
	if in.Args != nil {
		s.Args, _ = json.Marshal(in.Args)
	}
	if in.Env != nil {
		s.Env, _ = json.Marshal(in.Env)
	}
	return uc.repo.UpdateServer(ctx, s)
}

func (uc *McpUsecase) DeleteServer(ctx context.Context, id int64) error {
	if _, err := uc.repo.GetServer(ctx, id); err != nil {
		return ErrNotFound
	}
	return uc.repo.DeleteServer(ctx, id)
}

// ToggleServer 启停 MCP server 配置。
func (uc *McpUsecase) ToggleServer(ctx context.Context, id int64) (*po.McpServer, error) {
	s, err := uc.repo.GetServer(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	if s.Status == "enabled" {
		s.Status = "disabled"
	} else {
		s.Status = "enabled"
	}
	if err := uc.repo.UpdateServer(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// ---- 规则链暴露 ----

// ExposeChain 把已发布链注册为 MCP 工具。
// inputSchema 必填（生成物契约）：MCP 工具面向外部调用方，必须声明如何传参。
func (uc *McpUsecase) ExposeChain(ctx context.Context, chainID, toolName, description string, inputSchema json.RawMessage) (*po.McpExposure, error) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil, errors.New("toolName 不能为空")
	}
	if existing, err := uc.repo.GetExposureByTool(ctx, toolName); err == nil && existing.ID > 0 {
		return nil, fmt.Errorf("工具名 %q 已被占用", toolName)
	}
	if !hasSubstance(inputSchema) {
		return nil, errors.New("入参 schema 必填：请在规则链“链设置”中填写入参格式，或在暴露时提供")
	}
	e := &po.McpExposure{
		ChainID: chainID, ToolName: toolName, Description: description,
		InputSchema: datatypes.JSON(inputSchema), Enabled: true,
	}
	if err := uc.repo.CreateExposure(ctx, e); err != nil {
		return nil, err
	}
	uc.registerTool(e)
	return e, nil
}

// hasSubstance 判断 schema 是否有实质内容（非空、非 {}、非 null、且为合法 JSON）。
// 先 compact 去掉无意义空白，避免 " { } " 这类被漏判。
func hasSubstance(s json.RawMessage) bool {
	t := strings.TrimSpace(string(s))
	if t == "" || t == "null" {
		return false
	}
	if !json.Valid(s) {
		return false
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, s); err != nil {
		return false
	}
	return buf.String() != "{}" && buf.String() != "[]" && buf.String() != "null"
}

// ListExposures 列出全部已暴露工具。
func (uc *McpUsecase) ListExposures(ctx context.Context) ([]po.McpExposure, error) {
	return uc.repo.ListExposures(ctx)
}

// RemoveExposure 取消暴露（从 server 注销 + 删库）。
func (uc *McpUsecase) RemoveExposure(ctx context.Context, id int64) error {
	e, err := uc.repo.GetExposure(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	uc.mu.Lock()
	uc.mcpServer.DeleteTools(e.ToolName)
	delete(uc.loaded, e.ToolName)
	uc.mu.Unlock()
	return uc.repo.DeleteExposure(ctx, id)
}

// RestoreExposures 启动时把所有启用中的暴露工具重新注册到 MCP server。
func (uc *McpUsecase) RestoreExposures(ctx context.Context) error {
	list, err := uc.repo.ListEnabledExposures(ctx)
	if err != nil {
		return err
	}
	for i := range list {
		uc.registerTool(&list[i])
	}
	return nil
}

// registerTool 把一条 exposure 注册为 mcp-go 工具。
func (uc *McpUsecase) registerTool(e *po.McpExposure) {
	if !e.Enabled {
		return
	}
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.loaded[e.ToolName] {
		return
	}
	// 用 inputSchema 直接作为工具 schema（raw）
	tool := mcp.NewToolWithRawSchema(e.ToolName, e.Description, json.RawMessage(e.InputSchema))
	chainID := e.ChainID
	toolName := e.ToolName
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		args := req.GetArguments()
		data, _ := json.Marshal(args)
		out, err := runChainWithTrigger(ctx, uc.runner, chainID, string(data), "mcp")
		McpCallDuration.WithLabelValues(toolName).Observe(time.Since(start).Seconds())
		if err != nil {
			McpCallTotal.WithLabelValues(toolName, "error").Inc()
			return mcp.NewToolResultError(err.Error()), nil
		}
		McpCallTotal.WithLabelValues(toolName, "ok").Inc()
		return mcp.NewToolResultText(out), nil
	}
	uc.mcpServer.AddTool(tool, handler)
	uc.loaded[e.ToolName] = true
}

// toolNames 返回当前已注册工具名（调试用）。
func (uc *McpUsecase) toolNames() []string {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	out := make([]string, 0, len(uc.loaded))
	for n := range uc.loaded {
		out = append(out, n)
	}
	return out
}
