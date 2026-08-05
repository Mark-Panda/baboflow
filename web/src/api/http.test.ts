import { AxiosError, type AxiosRequestConfig, type AxiosResponse } from 'axios';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('antd', () => ({
  message: { error: vi.fn() },
}));

import { message } from 'antd';
import http from './http';

function mockResponse<T>(data: T, config: AxiosRequestConfig): AxiosResponse<T> {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config: config as AxiosResponse<T>['config'],
  };
}

describe('HTTP adapter', () => {
  afterEach(() => {
    vi.mocked(message.error).mockClear();
    vi.unstubAllGlobals();
  });

  it('returns proto JSON without Envelope unpacking', async () => {
    http.defaults.adapter = (config) =>
      Promise.resolve(mockResponse({ code: 0, message: 'proto field' }, config));

    await expect(http.get('/auth/me')).resolves.toEqual({
      code: 0,
      message: 'proto field',
    });
  });

  it.each([
    [{ message: 'message error', reason: 'reason error', code: 'CODE_ERROR' }, 'message error'],
    [{ reason: 'reason error', code: 'CODE_ERROR' }, 'reason error'],
    [{ code: 'CODE_ERROR' }, 'Request failed'],
  ])('uses message, reason, then Axios message as the error priority', async (body, expected) => {
    const response = mockResponse(body, {} as AxiosRequestConfig);
    response.status = 400;
    http.defaults.adapter = (config) =>
      Promise.reject(
        new AxiosError(
          'Request failed',
          undefined,
          config,
          undefined,
          { ...response, config: config as AxiosResponse['config'] },
        ),
      );

    await expect(http.get('/auth/me')).rejects.toMatchObject({
      code: 400,
      message: expected,
    });
    expect(message.error).toHaveBeenCalledWith(expected);
  });

  it('redirects HTTP 401 responses to login without showing an error toast', async () => {
    const location = { pathname: '/boards', href: '/boards' };
    vi.stubGlobal('location', location);
    const response = mockResponse({ message: 'unauthorized' }, {} as AxiosRequestConfig);
    response.status = 401;
    http.defaults.adapter = (config) =>
      Promise.reject(
        new AxiosError(
          'Request failed',
          undefined,
          config,
          undefined,
          { ...response, config: config as AxiosResponse['config'] },
        ),
      );

    await expect(http.get('/auth/me')).rejects.toMatchObject({
      code: 401,
      message: 'unauthorized',
    });
    expect(location.href).toBe('/login');
    expect(message.error).not.toHaveBeenCalled();
  });
});
