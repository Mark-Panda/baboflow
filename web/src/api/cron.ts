import http from './http';

// ---- Cron 定时任务 ----

export type CronScheduleType = 'once' | 'interval' | 'cron';
export type CronTargetType = 'chain' | 'agent';

export interface CronJob {
  id: number;
  name: string;
  targetType: CronTargetType;
  targetId: string;
  scheduleType: CronScheduleType;
  cronExpr: string;
  intervalSec: number;
  runAt?: string;
  payload?: Record<string, unknown>;
  enabled: boolean;
  lastRunAt?: string;
  lastStatus?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CronInput {
  name?: string;
  targetType: CronTargetType;
  targetId: string;
  scheduleType?: CronScheduleType;
  cronExpr?: string;
  intervalSec?: number;
  runAt?: string;
  payload?: Record<string, unknown>;
}

export const cronApi = {
  list: () => http.get<unknown, { list: CronJob[] }>('/cron'),
  create: (in_: CronInput) => http.post<unknown, CronJob>('/cron', in_),
  update: (id: number, in_: CronInput) => http.put<unknown, unknown>(`/cron/${id}`, in_),
  remove: (id: number) => http.delete<unknown, unknown>(`/cron/${id}`),
  toggle: (id: number) => http.post<unknown, CronJob>(`/cron/${id}/toggle`),
};
