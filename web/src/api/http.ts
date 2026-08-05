import axios, { AxiosError } from 'axios';
import { message } from 'antd';

// protojson 将 protobuf 的 int64/uint64 编码为十进制字符串，保留其全部精度。
export type ProtoInt64 = string;

export function toSafeNumber(value: ProtoInt64, field: string): number {
  const numberValue = Number(value);
  if (!Number.isSafeInteger(numberValue)) {
    throw new RangeError(`${field} exceeds JavaScript's safe integer range`);
  }
  return numberValue;
}

export interface KratosErrorBody {
  message?: string;
  reason?: string;
  code?: number | string;
}

export interface Page<T> {
  list: T[];
  page: {
    total: ProtoInt64;
    page: number;
    pageSize: number;
  };
}

export function toPageQuery<Params extends { page?: number; pageSize?: number }>(params: Params) {
  const { page, pageSize, ...filters } = params;
  return {
    ...filters,
    ...(page === undefined ? {} : { 'page.page': page }),
    ...(pageSize === undefined ? {} : { 'page.pageSize': pageSize }),
  };
}

const http = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  withCredentials: true, // 携带 HttpOnly Cookie Session
});

// 响应拦截：保留 proto JSON + 统一错误提示 + 401 跳登录
http.interceptors.response.use(
  (resp) => resp.data as never,
  (err: AxiosError<KratosErrorBody>) => {
    const status = err.response?.status;
    const body = err.response?.data;
    const msg = body?.message || body?.reason || err.message || '网络错误';
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
