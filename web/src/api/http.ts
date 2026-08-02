import axios, { AxiosError } from 'axios';
import { message } from 'antd';

// 后端统一信封 {code,message,data}
export interface Envelope<T = unknown> {
  code: number;
  message: string;
  data?: T;
}

export interface Page<T> {
  list: T[];
  total: number;
  page: number;
  pageSize: number;
}

const http = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  withCredentials: true, // 携带 HttpOnly Cookie Session
});

// 响应拦截：解信封 + 统一错误提示 + 401 跳登录
http.interceptors.response.use(
  (resp) => {
    const env = resp.data as Envelope;
    if (env && typeof env.code === 'number') {
      if (env.code === 0) {
        return env.data as never;
      }
      // 401 由下方 error 分支无法捕获（HTTP 200），这里处理
      if (env.code === 401) {
        redirectLogin();
        return Promise.reject(new ApiError(env.code, env.message));
      }
      message.error(env.message || '请求失败');
      return Promise.reject(new ApiError(env.code, env.message));
    }
    return resp.data as never;
  },
  (err: AxiosError<Envelope>) => {
    const status = err.response?.status;
    const msg = err.response?.data?.message || err.message || '网络错误';
    if (status === 401) redirectLogin();
    else message.error(msg);
    return Promise.reject(new ApiError(status ?? -1, msg));
  }
);

export class ApiError extends Error {
  code: number;
  constructor(code: number, msg: string) {
    super(msg);
    this.code = code;
  }
}

function redirectLogin() {
  if (location.pathname !== '/login') {
    location.href = '/login';
  }
}

export default http;
