import { describe, expect, it } from 'vitest';

import type { ComponentFormField } from '@/api/component';
import {
  optionCountOf,
  relationFor,
  useRadio,
  widgetFor,
} from './fieldWidgets';

const field = (partial: Partial<ComponentFormField>): ComponentFormField => ({
  name: 'x',
  type: 'string',
  ...partial,
});

describe('relationFor', () => {
  it('flow.targetId 映射到已发布规则链下拉', () => {
    const rel = relationFor('flow', 'targetId');
    expect(rel).toBeDefined();
    expect(rel?.api).toBe('chains');
    expect(rel?.valueKey).toBe('id');
    expect(rel?.labelKey).toBe('name');
    expect(rel?.params).toMatchObject({ status: 'published' });
  });

  it('agent.agentKey 映射到 Agent 下拉（存 key）', () => {
    const rel = relationFor('agent', 'agentKey');
    expect(rel?.api).toBe('agents');
    expect(rel?.valueKey).toBe('key');
    expect(rel?.labelKey).toBe('name');
  });

  it('agent.llmModelId 为两级级联（依赖 provider）', () => {
    const rel = relationFor('agent', 'llmModelId');
    expect(rel?.api).toBe('llmModels');
    expect(rel?.dependsOn).toBe('llmProviderId');
  });

  it('普通字段无关系映射', () => {
    expect(relationFor('log', 'jsScript')).toBeUndefined();
    expect(relationFor('restApiCall', 'body')).toBeUndefined();
  });
});

describe('useRadio', () => {
  it('requestMethod 少量选项用单选', () => {
    expect(useRadio('restApiCall', 'requestMethod', 4)).toBe(true);
  });

  it('选项超过上限回退下拉', () => {
    expect(useRadio('restApiCall', 'requestMethod', 6)).toBe(false);
  });

  it('未在名单内的字段不用单选', () => {
    expect(useRadio('log', 'jsScript', 2)).toBe(false);
  });

  it('0 个选项不用单选', () => {
    expect(useRadio('restApiCall', 'requestMethod', 0)).toBe(false);
  });
});

describe('widgetFor', () => {
  it('关系字段优先返回 relation', () => {
    expect(widgetFor('flow', field({ name: 'targetId' }))).toBe('relation');
    expect(widgetFor('agent', field({ name: 'agentKey' }))).toBe('relation');
  });

  it('headers/metadata/env 识别为键值对', () => {
    expect(widgetFor('restApiCall', field({ name: 'headers' }))).toBe('kv');
    expect(widgetFor('x', field({ name: 'env' }))).toBe('kv');
  });

  it('对象/数组类型识别为 json', () => {
    expect(widgetFor('x', field({ name: 'conf', type: 'object' }))).toBe('json');
    expect(widgetFor('x', field({ name: 'items', type: 'array' }))).toBe('json');
  });

  it('普通文本字段返回 undefined（走默认分派）', () => {
    expect(widgetFor('log', field({ name: 'jsScript' }))).toBeUndefined();
  });
});

describe('optionCountOf', () => {
  it('优先 component.options', () => {
    const f = field({
      name: 'requestMethod',
      component: {
        type: 'select',
        options: [
          { label: 'GET', value: 'GET' },
          { label: 'POST', value: 'POST' },
        ],
      },
    });
    expect(optionCountOf(f)).toBe(2);
  });

  it('回退 validate oneof=', () => {
    expect(optionCountOf(field({ name: 'm', validate: 'oneof=a b c' }))).toBe(3);
  });

  it('无选项返回 0', () => {
    expect(optionCountOf(field({ name: 'body' }))).toBe(0);
  });
});
