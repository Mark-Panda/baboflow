import type { AxiosRequestConfig, AxiosResponse } from 'axios';
import { afterEach, describe, expect, it } from 'vitest';
import { uploadAsset, assetUrl } from './agent';
import { auditApi } from './audit';
import { chainApi } from './chain';
import http from './http';
import { runApi } from './run';

function mockResponse<T>(data: T, config: AxiosRequestConfig): AxiosResponse<T> {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config: config as AxiosResponse<T>['config'],
  };
}

describe('API contracts', () => {
  afterEach(() => {
    http.defaults.adapter = undefined;
  });

  it.each([
    ['rule chains', () => chainApi.list({ status: 'draft', page: 2, pageSize: 25 })],
    ['rule chain runs', () => runApi.list({ chainId: 'chain-1', page: 3, pageSize: 50 })],
    ['audit logs', () => auditApi.list({ action: 'auth.login', page: 4, pageSize: 100 })],
  ])('serializes %s pagination using nested proto query fields', async (_name, request) => {
    let requestUri = '';
    http.defaults.adapter = (config) => {
      requestUri = http.getUri(config);
      return Promise.resolve(mockResponse({ list: [], page: { total: 0 } }, config));
    };

    await request();

    const query = new URL(requestUri, 'http://localhost').searchParams;
    expect(query.get('page.page')).not.toBeNull();
    expect(query.get('page.pageSize')).not.toBeNull();
    expect(query.has('page')).toBe(false);
    expect(query.has('pageSize')).toBe(false);
  });

  it('uses the registered Agent asset sidecar paths', async () => {
    let method = '';
    let url = '';
    http.defaults.adapter = (config) => {
      method = config.method ?? '';
      url = config.url ?? '';
      return Promise.resolve(mockResponse({ id: '9007199254740993', size: '5' }, config));
    };

    const asset = await uploadAsset('session-1', new File(['asset'], 'asset.txt', { type: 'text/plain' }));

    expect(method).toBe('post');
    expect(url).toBe('/agent-assets');
    expect(asset.id).toBe('9007199254740993');
    expect(asset.size).toBe('5');
    expect(assetUrl(asset.id)).toBe('/api/v1/agent-assets/9007199254740993');
  });
});
