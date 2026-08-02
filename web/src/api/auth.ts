import http from './http';

export interface CurrentUser {
  id: number;
  username: string;
  displayName: string;
  mustChangePwd: boolean;
}

export const authApi = {
  login: (username: string, password: string) =>
    http.post<unknown, CurrentUser>('/auth/login', { username, password }),
  logout: () => http.post<unknown, { ok: boolean }>('/auth/logout'),
  me: () => http.get<unknown, CurrentUser>('/auth/me'),
  changePassword: (oldPassword: string, newPassword: string) =>
    http.put<unknown, { ok: boolean }>('/auth/password', { oldPassword, newPassword }),
};
