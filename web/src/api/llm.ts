import http, { ProtoInt64 } from './http';

export interface Provider {
  id: ProtoInt64;
  name: string;
  provider: string;
  baseUrl: string;
  apiKeyMasked: string;
  extra: Record<string, unknown>;
  remark: string;
  modelCount: number;
}

export interface ProviderInput {
  name: string;
  provider?: string;
  baseUrl: string;
  apiKey?: string;
  extra?: Record<string, unknown>;
  remark?: string;
}

export interface LLMModel {
  id: ProtoInt64;
  providerId: ProtoInt64;
  model: string;
  alias: string;
  temperature: number;
  maxTokens: number;
  isDefault: boolean;
  capability: Record<string, unknown>;
  enabled: boolean;
}

export interface ModelInput {
  model: string;
  alias?: string;
  temperature?: number;
  maxTokens?: number;
  isDefault?: boolean;
  capability?: Record<string, unknown>;
  enabled?: boolean;
}

export interface TestResult {
  ok: boolean;
  latencyMs?: ProtoInt64;
  message?: string;
}

export const llmApi = {
  // 接入点
  listProviders: () => http.get<unknown, { list: Provider[] }>('/llm/providers'),
  createProvider: (in_: ProviderInput) => http.post<unknown, { id: ProtoInt64 }>('/llm/providers', in_),
  updateProvider: (id: ProtoInt64, in_: ProviderInput) => http.put<unknown, unknown>(`/llm/providers/${id}`, in_),
  deleteProvider: (id: ProtoInt64) => http.delete<unknown, unknown>(`/llm/providers/${id}`),
  testProvider: (id: ProtoInt64) => http.post<unknown, TestResult>(`/llm/providers/${id}/test`),
  remoteModels: (id: ProtoInt64) => http.get<unknown, { models: string[] }>(`/llm/providers/${id}/models/remote`),

  // 模型
  listModels: (providerId: ProtoInt64) => http.get<unknown, { list: LLMModel[] }>(`/llm/providers/${providerId}/models`),
  createModels: (providerId: ProtoInt64, models: ModelInput[]) =>
    http.post<unknown, unknown>(`/llm/providers/${providerId}/models`, { models }),
  updateModel: (modelId: ProtoInt64, in_: ModelInput) => http.put<unknown, unknown>(`/llm/models/${modelId}`, in_),
  deleteModel: (modelId: ProtoInt64) => http.delete<unknown, unknown>(`/llm/models/${modelId}`),
  setDefaultModel: (modelId: ProtoInt64) => http.post<unknown, unknown>(`/llm/models/${modelId}/default`),
  testModel: (modelId: ProtoInt64) => http.post<unknown, TestResult>(`/llm/models/${modelId}/test`),
};
