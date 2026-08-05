import http, { ProtoInt64 } from './http';

// Archery 连接：只含地址+凭据；实例由"更新实例"从 Archery 拉取，不在此填写。
// 凭据加密存库，密码仅回脱敏掩码。
export interface ArcheryConnection {
  id: ProtoInt64;
  name: string;
  endpoint: string;
  username: string;
  password: string; // 脱敏掩码（如 abc****wxyz）
  insecure: boolean;
  caCert: string;
  remark: string;
  instanceCount: number; // 该连接下已同步的实例数
  isDefault: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface ArcheryConnectionInput {
  name: string;
  endpoint: string;
  username: string;
  password?: string; // 创建必填；更新留空=不修改
  insecure?: boolean;
  caCert?: string;
  remark?: string;
}

// Archery 实例：某连接下一个可查询数据源（由同步产生）。
export interface ArcheryInstance {
  id: ProtoInt64;
  connectionId: ProtoInt64;
  instanceName: string;
  dbType: string;
}

export interface ArcheryTestResult {
  ok: boolean;
  instances?: number; // 登录成功后拉到的实例数
  error?: string;
}

export const archeryApi = {
  listConnections: () => http.get<unknown, { list: ArcheryConnection[] }>('/archery/connections'),
  getConnection: (id: ProtoInt64) => http.get<unknown, ArcheryConnection>(`/archery/connections/${id}`),
  createConnection: (in_: ArcheryConnectionInput) =>
    http.post<unknown, { id: ProtoInt64 }>('/archery/connections', in_),
  updateConnection: (id: ProtoInt64, in_: ArcheryConnectionInput) =>
    http.put<unknown, unknown>(`/archery/connections/${id}`, in_),
  deleteConnection: (id: ProtoInt64) => http.delete<unknown, unknown>(`/archery/connections/${id}`),
  setDefaultConnection: (id: ProtoInt64) => http.post<unknown, unknown>(`/archery/connections/${id}/default`),
  clearDefaultConnection: (id: ProtoInt64) => http.delete<unknown, unknown>(`/archery/connections/${id}/default`),
  testConnection: (id: ProtoInt64) =>
    http.post<unknown, ArcheryTestResult>(`/archery/connections/${id}/test`),
  listInstances: (connectionId: ProtoInt64) =>
    http.get<unknown, { list: ArcheryInstance[] }>(`/archery/connections/${connectionId}/instances`),
  syncInstances: (connectionId: ProtoInt64) =>
    http.post<unknown, { list: ArcheryInstance[] }>(`/archery/connections/${connectionId}/sync-instances`),
};
