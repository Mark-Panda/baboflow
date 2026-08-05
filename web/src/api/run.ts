import http, { Page, ProtoInt64, toPageQuery } from './http';
import { NodeTrace } from './chain';

export interface ChainRun {
  id: ProtoInt64;
  chainId: string;
  trigger: string;
  status: string;
  input: unknown;
  output: unknown;
  error: string;
  nodeTrace: NodeTrace[];
  startedAt: string;
  finishedAt?: string;
}

export const runApi = {
  list: (params: { chainId?: string; status?: string; page?: number; pageSize?: number }) =>
    http.get<unknown, Page<ChainRun>>('/rule-chain-runs', { params: toPageQuery(params) }),
  detail: (runId: ProtoInt64) => http.get<unknown, ChainRun>(`/rule-chain-runs/${runId}`),
};
