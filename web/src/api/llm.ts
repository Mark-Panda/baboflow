import http from './http';

export interface Provider {
  id: number;
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
  id: number;
  providerId: number;
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
  latencyMs?: number;
  message?: string;
}

export const llmApi = {
  // 接入点
  listProviders: () => http.get<unknown, { list: Provider[] }>('/llm/providers'),
  createProvider: (in_: ProviderInput) => http.post<unknown, { id: number }>('/llm/providers', in_),
  updateProvider: (id: number, in_: ProviderInput) => http.put<unknown, unknown>(`/llm/providers/${id}`, in_),
  deleteProvider: (id: number) => http.delete<unknown, unknown>(`/llm/providers/${id}`),
  testProvider: (id: number) => http.post<unknown, TestResult>(`/llm/providers/${id}/test`),
  remoteModels: (id: number) => http.get<unknown, { models: string[] }>(`/llm/providers/${id}/models/remote`),

  // 模型
  listModels: (providerId: number) => http.get<unknown, { list: LLMModel[] }>(`/llm/providers/${providerId}/models`),
  createModels: (providerId: number, models: ModelInput[]) =>
    http.post<unknown, unknown>(`/llm/providers/${providerId}/models`, { models }),
  updateModel: (modelId: number, in_: ModelInput) => http.put<unknown, unknown>(`/llm/models/${modelId}`, in_),
  deleteModel: (modelId: number) => http.delete<unknown, unknown>(`/llm/models/${modelId}`),
  setDefaultModel: (modelId: number) => http.post<unknown, unknown>(`/llm/models/${modelId}/default`),
  testModel: (modelId: number) => http.post<unknown, TestResult>(`/llm/models/${modelId}/test`),
};
