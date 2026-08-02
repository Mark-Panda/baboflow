import http from './http';

// ---- MCP Server 配置 ----

export type McpTransport = 'sse' | 'stdio' | 'streamable-http';

export interface McpServer {
  id: number;
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
  id: number;
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
  updateServer: (id: number, in_: McpServerInput) => http.put<unknown, unknown>(`/mcp/servers/${id}`, in_),
  removeServer: (id: number) => http.delete<unknown, unknown>(`/mcp/servers/${id}`),
  toggleServer: (id: number) => http.post<unknown, McpServer>(`/mcp/servers/${id}/toggle`),
  testServer: (id: number) =>
    http.post<unknown, { ok: boolean; tools?: string[]; error?: string }>(`/mcp/servers/${id}/test`),

  // 规则链暴露
  listExposures: () => http.get<unknown, { list: McpExposure[] }>('/mcp/exposures'),
  expose: (chainId: string, in_: ExposeInput) =>
    http.post<unknown, { id: number; toolName: string; mcpEndpoint: string }>(
      `/chains/${encodeURIComponent(chainId)}/expose`, in_),
  removeExposure: (id: number) => http.delete<unknown, unknown>(`/mcp/exposures/${id}`),
};
