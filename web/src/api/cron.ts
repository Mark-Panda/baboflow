import http, { ProtoInt64 } from './http';

// ---- Cron 定时任务 ----

export type CronScheduleType = 'once' | 'interval' | 'cron';
export type CronTargetType = 'chain' | 'agent';

export interface CronJob {
  id: ProtoInt64;
  name: string;
  targetType: CronTargetType;
  targetId: string;
  scheduleType: CronScheduleType;
  cronExpr: string;
  intervalSec: ProtoInt64;
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
  intervalSec?: ProtoInt64;
  runAt?: string;
  payload?: Record<string, unknown>;
}

export const cronApi = {
  list: () => http.get<unknown, { list: CronJob[] }>('/cron-jobs'),
  create: (in_: CronInput) => http.post<unknown, CronJob>('/cron-jobs', in_),
  update: (id: ProtoInt64, in_: CronInput) => http.put<unknown, unknown>(`/cron-jobs/${id}`, in_),
  remove: (id: ProtoInt64) => http.delete<unknown, unknown>(`/cron-jobs/${id}`),
  toggle: (id: ProtoInt64) => http.post<unknown, CronJob>(`/cron-jobs/${id}/toggle`),
};
