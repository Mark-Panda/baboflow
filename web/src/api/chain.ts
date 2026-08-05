import http, { Page, ProtoInt64, toPageQuery } from './http';

export interface RuleChain {
  id: string;
  name: string;
  description: string;
  inputSchema?: Record<string, unknown>;
  status: 'draft' | 'published' | 'archived';
  version: number;
  source: string;
  debugMode: boolean;
  dsl: unknown;
  updatedAt: string;
  createdAt: string;
}

export interface ChainListItem {
  id: string;
  name: string;
  description: string;
  status: string;
  version: number;
  source: string;
  debugMode: boolean;
  updatedAt: string;
}

export interface ChainInput {
  name: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
  dsl?: unknown;
  debugMode?: boolean;
  source?: string;
}

export interface ChainVersion {
  id: ProtoInt64;
  chainId: string;
  version: number;
  dsl: unknown;
  publishedAt: string;
}

export interface NodeTrace {
  nodeId: string;
  flowType: string;
  relationType: string;
  data: string;
  in?: string;
  out?: string;
  input?: TraceMessage;
  output?: TraceMessage;
  durationMs?: ProtoInt64;
  err?: string;
}

export interface TraceMessage {
  msg: string;
  metadata: Record<string, string>;
  type: string;
  dataType: string;
}

export interface RunResult {
  runId: ProtoInt64;
  status: string;
  output: string;
  error: string;
  nodeTrace: NodeTrace[];
}

export interface RunInput {
  dataType?: string;
  data: string;
  metadata?: Record<string, string>;
}

export interface ValidateResult {
  valid: boolean;
  error?: string;
}

export const chainApi = {
  list: (params: { status?: string; keyword?: string; page?: number; pageSize?: number }) =>
    http.get<unknown, Page<ChainListItem>>('/rule-chains', { params: toPageQuery(params) }),
  get: (id: string) => http.get<unknown, RuleChain>(`/rule-chains/${id}`),
  create: (in_: ChainInput) => http.post<unknown, RuleChain>('/rule-chains', in_),
  update: (id: string, in_: ChainInput) => http.put<unknown, unknown>(`/rule-chains/${id}`, in_),
  remove: (id: string) => http.delete<unknown, unknown>(`/rule-chains/${id}`),
  validate: (dsl: unknown) => http.post<unknown, ValidateResult>('/rule-chains/validate', { dsl }),
  publish: (id: string) => http.post<unknown, { version: number }>(`/rule-chains/${id}/publish`),
  offline: (id: string) => http.post<unknown, unknown>(`/rule-chains/${id}/offline`),
  versions: (id: string) => http.get<unknown, { list: ChainVersion[] }>(`/rule-chains/${id}/versions`),
  rollback: (id: string, version: number) => http.post<unknown, unknown>(`/rule-chains/${id}/rollback`, { version }),
  export: (id: string) => http.get<unknown, { name: string; description: string; version: number; dsl: unknown }>(`/rule-chains/${id}/export`),
  import: (in_: { name: string; description?: string; dsl: unknown }) =>
    http.post<unknown, RuleChain>('/rule-chains/import', in_),
  run: (id: string, in_: RunInput) => http.post<unknown, RunResult>(`/rule-chains/${id}/run`, in_),
  debug: (id: string, in_: RunInput) => http.post<unknown, RunResult>(`/rule-chains/${id}/debug`, in_),
};
