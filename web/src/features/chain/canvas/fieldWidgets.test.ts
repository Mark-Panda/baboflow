import { describe, expect, it } from 'vitest';

import type { ComponentFormField } from '@/api/component';
import {
  isFreeInputSelect,
  isHiddenField,
  isStructArrayField,
  optionCountOf,
  relationFor,
  staticOptionsFor,
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

  it('节点引用字段映射到本链节点选择器', () => {
    // 单选 + 允许手输跨链
    const ref = relationFor('ref', 'targetId');
    expect(ref?.api).toBe('nodes');
    expect(ref?.freeInput).toBe(true);
    expect(ref?.multiple).toBeFalsy();

    // 单选（不允许跨链）
    const fetch = relationFor('fetchNodeOutput', 'nodeId');
    expect(fetch?.api).toBe('nodes');
    expect(fetch?.freeInput).toBeFalsy();

    // 循环体节点（允许手输 chain:{chainId}）
    expect(relationFor('for', 'do')?.api).toBe('nodes');
    expect(relationFor('while', 'do')?.api).toBe('nodes');
  });

  it('nodeIds 为多选节点选择器', () => {
    expect(relationFor('groupAction', 'nodeIds')).toMatchObject({
      api: 'nodes',
      multiple: true,
    });
    expect(relationFor('groupFilter', 'nodeIds')).toMatchObject({
      api: 'nodes',
      multiple: true,
    });
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

  it('mapping（字段→表达式）识别为键值对', () => {
    expect(widgetFor('exprTransform', field({ name: 'mapping', type: 'map' }))).toBe('kv');
    expect(widgetFor('metadataTransform', field({ name: 'mapping', type: 'map' }))).toBe('kv');
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

describe('staticOptionsFor', () => {
  it('dbClient.opType 提供 SELECT/INSERT/UPDATE/DELETE', () => {
    const opts = staticOptionsFor('dbClient', 'opType');
    expect(opts?.map((o) => o.value)).toEqual([
      'SELECT',
      'INSERT',
      'UPDATE',
      'DELETE',
    ]);
  });

  it('mqttClient.qos 为数字 0/1/2', () => {
    expect(staticOptionsFor('mqttClient', 'qos')?.map((o) => o.value)).toEqual([
      0, 1, 2,
    ]);
  });

  it('net.protocol 为 tcp/udp', () => {
    expect(staticOptionsFor('net', 'protocol')?.map((o) => o.value)).toEqual([
      'tcp',
      'udp',
    ]);
  });

  it('for.mode 为 0-3、while.mode 为 0-2', () => {
    expect(staticOptionsFor('for', 'mode')?.map((o) => o.value)).toEqual([
      0, 1, 2, 3,
    ]);
    expect(staticOptionsFor('while', 'mode')?.map((o) => o.value)).toEqual([
      0, 1, 2,
    ]);
  });

  it('非枚举字段返回 undefined', () => {
    expect(staticOptionsFor('restApiCall', 'body')).toBeUndefined();
    expect(staticOptionsFor('log', 'jsScript')).toBeUndefined();
  });

  it('少量选项命中 RADIO_FIELDS 时用单选', () => {
    expect(useRadio('net', 'protocol', staticOptionsFor('net', 'protocol')!.length)).toBe(true);
    expect(useRadio('mqttClient', 'qos', staticOptionsFor('mqttClient', 'qos')!.length)).toBe(true);
    expect(useRadio('dbClient', 'opType', staticOptionsFor('dbClient', 'opType')!.length)).toBe(true);
  });
});

describe('isFreeInputSelect', () => {
  it('groupAction.matchRelationType 允许自定义', () => {
    expect(isFreeInputSelect('groupAction', 'matchRelationType')).toBe(true);
  });
  it('其它字段不可自由输入', () => {
    expect(isFreeInputSelect('dbClient', 'opType')).toBe(false);
  });
});

describe('isHiddenField', () => {
  it('delay 的 deprecated 字段被隐藏', () => {
    expect(isHiddenField('delay', 'periodInSeconds')).toBe(true);
    expect(isHiddenField('delay', 'periodInSecondsPattern')).toBe(true);
  });
  it('普通字段不隐藏', () => {
    expect(isHiddenField('delay', 'delayMs')).toBe(false);
    expect(isHiddenField('restApiCall', 'body')).toBe(false);
  });
});

describe('isStructArrayField', () => {
  it('cache 系列的 keys/items 是 struct 数组', () => {
    expect(isStructArrayField('cacheGet', 'keys')).toBe(true);
    expect(isStructArrayField('cacheSet', 'items')).toBe(true);
    expect(isStructArrayField('cacheDelete', 'keys')).toBe(true);
  });
  it('switch.cases 不在这里接管（有专用编辑器）', () => {
    expect(isStructArrayField('switch', 'cases')).toBe(false);
  });
  it('普通字段不是 struct 数组', () => {
    expect(isStructArrayField('restApiCall', 'headers')).toBe(false);
  });
});
