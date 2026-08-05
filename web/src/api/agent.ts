import http, { ProtoInt64 } from './http';

// ---- 类型 ----

export interface Agent {
  id: ProtoInt64;
  key: string;
  name: string;
  instruction: string;
  llmModelId?: ProtoInt64;
  skillIds: ProtoInt64[];
  mcpIds: ProtoInt64[];
  builtinTools: string[];
  subAgentIds: ProtoInt64[];
  isBuiltin: boolean;
  enabled: boolean;
  updatedAt: string;
}

export interface AgentInput {
  name: string;
  instruction?: string;
  llmModelId?: ProtoInt64;
  skillIds?: ProtoInt64[];
  mcpIds?: ProtoInt64[];
  builtinTools?: string[];
  subAgentIds?: ProtoInt64[];
  enabled?: boolean;
}

export interface AgentSession {
  id: string;
  agentKey: string;
  chainId?: string;
  title: string;
  createdAt: string;
  updatedAt: string;
}

export interface ToolCallRec {
  name: string;
  input: string;
  output?: string;
  status: 'ok' | 'error';
  questionId?: string;
  question?: AgentQuestion;
}

export interface AgentQuestion {
  question: string;
  options: string[];
  multiple?: boolean;
  allowOther?: boolean;
}

export interface AttachmentRef {
  assetId: ProtoInt64;
  name: string;
  mime: string;
}

export interface AgentMessage {
  id: ProtoInt64;
  sessionId: string;
  role: 'user' | 'assistant' | 'tool' | 'system';
  content: string;
  toolCalls?: ToolCallRec[];
  attachment?: AttachmentRef[];
  subAgent?: string;
  createdAt: string;
}

export interface Asset {
  id: ProtoInt64;
  name: string;
  mime: string;
  size: ProtoInt64;
  sessionId: string;
  createdAt: string;
}

// ---- Agent CRUD ----

export function listAgents(keyword?: string): Promise<{ list: Agent[] }> {
  return http.get('/agents', { params: { keyword } }) as never;
}

export function getAgent(key: string): Promise<Agent> {
  return http.get(`/agents/${encodeURIComponent(key)}`) as never;
}

export function createAgent(key: string, input: AgentInput): Promise<Agent> {
  return http.post('/agents', { key, ...input }) as never;
}

export function updateAgent(key: string, input: AgentInput): Promise<{ ok: boolean }> {
  return http.put(`/agents/${encodeURIComponent(key)}`, input) as never;
}

export function deleteAgent(key: string): Promise<{ ok: boolean }> {
  return http.delete(`/agents/${encodeURIComponent(key)}`) as never;
}

// ---- 会话 ----

export function listSessions(agentKey: string): Promise<{ list: AgentSession[] }> {
  return http.get('/agent-sessions', { params: { agentKey } }) as never;
}

export function createSession(agentKey: string, title?: string, chainId?: string): Promise<AgentSession> {
  return http.post('/agent-sessions', { agentKey, title, chainId }) as never;
}

export function deleteSession(sessionId: string): Promise<{ ok: boolean }> {
  return http.delete(`/agent-sessions/${encodeURIComponent(sessionId)}`) as never;
}

export function listMessages(sessionId: string): Promise<{ list: AgentMessage[] }> {
  return http.get(`/agent-sessions/${encodeURIComponent(sessionId)}/messages`) as never;
}

// ---- 附件 ----

export function uploadAsset(sessionId: string, file: File): Promise<Asset> {
  const fd = new FormData();
  fd.append('sessionId', sessionId);
  fd.append('file', file);
  return http.post('/agent-assets', fd, {
    headers: { 'Content-Type': 'multipart/form-data' },
  }) as never;
}

export function assetUrl(assetId: ProtoInt64): string {
  return `/api/v1/agent-assets/${assetId}`;
}
