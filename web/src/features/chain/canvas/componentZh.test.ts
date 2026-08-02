import { describe, expect, it } from 'vitest';

import { componentZhName, componentZhDesc, fieldZh, COMPONENT_ZH } from './componentZh';

describe('componentZh 中文映射', () => {
  it('已知组件返回中文名', () => {
    expect(componentZhName('jsTransform')).toBe('JS 转换');
    expect(componentZhName('restApiCall')).toBe('HTTP 请求');
    expect(componentZhName('dbClient')).toBe('数据库');
    expect(componentZhName('switch')).toBe('条件分支');
  });

  it('未知组件回退为原 type', () => {
    expect(componentZhName('no/such/thing')).toBe('no/such/thing');
  });

  it('组件中文描述可获取', () => {
    expect(componentZhDesc('jsFilter')).toContain('过滤');
    expect(componentZhDesc('unknown/type')).toBeUndefined();
  });

  it('JS 脚本字段有中文 label/desc', () => {
    expect(fieldZh('jsTransform', 'jsScript')?.label).toBe('转换脚本');
    expect(fieldZh('jsFilter', 'jsScript')?.label).toBe('过滤脚本');
    expect(fieldZh('jsSwitch', 'jsScript')?.label).toBe('路由脚本');
    expect(fieldZh('log', 'jsScript')?.label).toBe('日志脚本');
  });

  it('未映射字段返回 undefined', () => {
    expect(fieldZh('jsTransform', 'nope')).toBeUndefined();
    expect(fieldZh('unknown/type', 'jsScript')).toBeUndefined();
  });

  it('所有映射都带中文名', () => {
    Object.values(COMPONENT_ZH).forEach((c) => {
      expect(c.name.trim().length).toBeGreaterThan(0);
    });
  });
});
