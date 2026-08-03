import http from './http';

// Archery 连接（凭据加密存库，密码仅回脱敏掩码）。
export interface ArcheryConnection {
  id: number;
  name: string;
  endpoint: string;
  instance: string;
  username: string;
  password: string; // 脱敏掩码（如 abc****wxyz）
  insecure: boolean;
  caCert: string;
  remark: string;
  createdAt: string;
  updatedAt: string;
}

export interface ArcheryConnectionInput {
  name: string;
  endpoint: string;
  instance: string;
  username: string;
  password?: string; // 创建必填；更新留空=不修改
  insecure?: boolean;
  caCert?: string;
  remark?: string;
}

export interface ArcheryTestResult {
  ok: boolean;
  instance?: string;
  databases?: string[];
  error?: string;
}

export const archeryApi = {
  listConnections: () => http.get<unknown, { list: ArcheryConnection[] }>('/archery/connections'),
  getConnection: (id: number) => http.get<unknown, ArcheryConnection>(`/archery/connections/${id}`),
  createConnection: (in_: ArcheryConnectionInput) =>
    http.post<unknown, { id: number }>('/archery/connections', in_),
  updateConnection: (id: number, in_: ArcheryConnectionInput) =>
    http.put<unknown, unknown>(`/archery/connections/${id}`, in_),
  deleteConnection: (id: number) => http.delete<unknown, unknown>(`/archery/connections/${id}`),
  testConnection: (id: number) =>
    http.post<unknown, ArcheryTestResult>(`/archery/connections/${id}/test`),
};
