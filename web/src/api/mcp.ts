import http, { ProtoInt64 } from './http';

// ---- MCP Server 配置 ----

export type McpTransport = 'sse' | 'stdio' | 'streamable-http';

export interface McpServer {
  id: ProtoInt64;
  name: string;
  transport: McpTransport;
  endpoint: string;
  command: string;
  args: string[];
  env?: Record<string, string>;
  status: 'enabled' | 'disabled' | 'error' | string;
  lastCheckAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface McpServerInput {
  name: string;
  transport?: McpTransport;
  endpoint?: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
}

// ---- MCP 规则链暴露 ----

export interface McpExposure {
  id: ProtoInt64;
  chainId: string;
  toolName: string;
  description: string;
  inputSchema?: Record<string, unknown>;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface ExposeInput {
  toolName: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
}

export const mcpApi = {
  // server 配置
  listServers: () => http.get<unknown, { list: McpServer[] }>('/mcp/servers'),
  createServer: (in_: McpServerInput) => http.post<unknown, McpServer>('/mcp/servers', in_),
  updateServer: (id: ProtoInt64, in_: McpServerInput) => http.put<unknown, unknown>(`/mcp/servers/${id}`, in_),
  removeServer: (id: ProtoInt64) => http.delete<unknown, unknown>(`/mcp/servers/${id}`),
  toggleServer: (id: ProtoInt64) => http.post<unknown, McpServer>(`/mcp/servers/${id}/toggle`),
  testServer: (id: ProtoInt64) =>
    http.post<unknown, { ok: boolean; tools?: string[]; error?: string }>(`/mcp/servers/${id}/test`),

  // 规则链暴露
  listExposures: () => http.get<unknown, { list: McpExposure[] }>('/mcp/exposures'),
  expose: (chainId: string, in_: ExposeInput) =>
    http.post<unknown, { id: ProtoInt64; toolName: string; mcpEndpoint: string }>(
      `/rule-chains/${encodeURIComponent(chainId)}/expose`, in_),
  removeExposure: (id: ProtoInt64) => http.delete<unknown, unknown>(`/mcp/exposures/${id}`),
};
