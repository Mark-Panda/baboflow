import http from './http';

export interface CurrentUser {
  id: number;
  username: string;
  displayName: string;
  mustChangePwd: boolean;
  avatar?: string;
  email?: string;
}

// 飞书登录入口（整页跳转，由后端 302 到飞书授权页）。
export const feishuLoginUrl = '/api/v1/auth/feishu/login';

export const authApi = {
  login: (username: string, password: string) =>
    http.post<unknown, CurrentUser>('/auth/login', { username, password }),
  logout: () => http.post<unknown, { ok: boolean }>('/auth/logout'),
  me: () => http.get<unknown, CurrentUser>('/auth/me'),
  changePassword: (oldPassword: string, newPassword: string) =>
    http.put<unknown, { ok: boolean }>('/auth/password', { oldPassword, newPassword }),
};
