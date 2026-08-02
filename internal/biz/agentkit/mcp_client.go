package agentkit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"gorm.io/datatypes"

	"baboflow/internal/data/po"
)

// McpClientBuilder 把 mcp_server 配置转成 eino 工具集（供 Agent 调用远程 MCP 工具）。
// 连接带超时，失败返回错误（上层可选择跳过该 server 或整体失败）。
type McpClientBuilder struct {
	connectTimeout time.Duration
}

func NewMcpClientBuilder() *McpClientBuilder {
	return &McpClientBuilder{connectTimeout: 10 * time.Second}
}

// BuildTools 连接一个 MCP server 并返回其全部工具。
func (b *McpClientBuilder) BuildTools(ctx context.Context, srv *po.McpServer) ([]tool.BaseTool, error) {
	tools, _, err := b.BuildToolsWithClose(ctx, srv)
	return tools, err
}

// BuildToolsWithClose 连接一个 MCP server 并返回其全部工具，同时返回 close 用于释放底层连接
// （stdio 子进程 / SSE / HTTP keep-alive）。工具在 close 后不可再用；调用方负责在不再需要时调用 close
// （如 agent 重建/删除、连通性测试结束），否则连接会泄露。
func (b *McpClientBuilder) BuildToolsWithClose(ctx context.Context, srv *po.McpServer) ([]tool.BaseTool, func(), error) {
	ctx, cancel := context.WithTimeout(ctx, b.connectTimeout)
	defer cancel()

	cli, err := b.dial(ctx, srv)
	if err != nil {
		return nil, nil, err
	}
	// Initialize handshake
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "baboflow", Version: "0.1.0"}
	if _, err := cli.Initialize(ctx, initReq); err != nil {
		_ = cli.Close()
		return nil, nil, fmt.Errorf("MCP 握手失败: %w", err)
	}
	tools, err := einomcp.GetTools(ctx, &einomcp.Config{Cli: cli})
	if err != nil {
		_ = cli.Close()
		return nil, nil, err
	}
	return tools, func() { _ = cli.Close() }, nil
}

// ListToolNames 连通性检测：连接并列出远程工具名（不保留连接）。
func (b *McpClientBuilder) ListToolNames(ctx context.Context, srv *po.McpServer) ([]string, error) {
	tools, closeFn, err := b.BuildToolsWithClose(ctx, srv)
	if err != nil {
		return nil, err
	}
	defer closeFn() // 连通性检测即连即断，避免泄露连接/子进程
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err == nil {
			names = append(names, info.Name)
		}
	}
	return names, nil
}

// dial 按 transport 建立 MCP 客户端连接。
func (b *McpClientBuilder) dial(ctx context.Context, srv *po.McpServer) (mcpclient.MCPClient, error) {
	env := mcpParseStrMap(srv.Env)
	switch srv.Transport {
	case "stdio":
		args := mcpParseStrList(srv.Args)
		envList := make([]string, 0, len(env))
		for k, v := range env {
			envList = append(envList, k+"="+v)
		}
		cli, err := mcpclient.NewStdioMCPClient(srv.Command, envList, args...)
		if err != nil {
			return nil, fmt.Errorf("stdio 连接失败: %w", err)
		}
		if err := cli.Start(ctx); err != nil {
			return nil, fmt.Errorf("stdio 启动失败: %w", err)
		}
		return cli, nil
	case "sse":
		cli, err := mcpclient.NewSSEMCPClient(srv.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("sse 连接失败: %w", err)
		}
		if err := cli.Start(ctx); err != nil {
			return nil, fmt.Errorf("sse 启动失败: %w", err)
		}
		return cli, nil
	case "streamable-http", "http":
		cli, err := mcpclient.NewStreamableHttpClient(srv.Endpoint,
			transport.WithHTTPHeaders(env))
		if err != nil {
			return nil, fmt.Errorf("streamable-http 连接失败: %w", err)
		}
		if err := cli.Start(ctx); err != nil {
			return nil, fmt.Errorf("streamable-http 启动失败: %w", err)
		}
		return cli, nil
	default:
		return nil, fmt.Errorf("不支持的 transport: %s", srv.Transport)
	}
}

func mcpParseStrList(j datatypes.JSON) []string {
	var s []string
	if len(j) > 0 {
		_ = json.Unmarshal(j, &s)
	}
	return s
}

func mcpParseStrMap(j datatypes.JSON) map[string]string {
	m := map[string]string{}
	if len(j) > 0 {
		_ = json.Unmarshal(j, &m)
	}
	return m
}
