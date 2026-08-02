import http, { Page } from './http';
import { NodeTrace } from './chain';

export interface ChainRun {
  id: number;
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
    http.get<unknown, Page<ChainRun>>('/chains/runs', { params }),
  detail: (runId: number) => http.get<unknown, ChainRun>(`/chains/runs/${runId}`),
};
