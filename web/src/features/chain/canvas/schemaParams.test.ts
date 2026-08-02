import { describe, expect, it } from 'vitest';

import {
  parseJsonc,
  rowsToSchema,
  schemaToRows,
  stripJsonComments,
  type ParamRow,
} from './schemaParams';

function row(partial: Partial<ParamRow>): ParamRow {
  return {
    key: partial.key ?? `k_${partial.name ?? Math.random()}`,
    name: partial.name ?? '',
    type: partial.type ?? 'string',
    required: partial.required ?? false,
    default: partial.default,
    description: partial.description,
  };
}

describe('rowsToSchema', () => {
  it('空行/无有效参数 → undefined（未声明契约）', () => {
    expect(rowsToSchema([])).toBeUndefined();
    expect(rowsToSchema([row({ name: '   ' })])).toBeUndefined();
  });

  it('生成合法 object schema，含 type/description/required', () => {
    const schema = rowsToSchema([
      row({ name: 't', type: 'number', required: true, description: '温度' }),
      row({ name: 'name', type: 'string' }),
    ]);
    expect(schema).toEqual({
      type: 'object',
      properties: {
        t: { type: 'number', description: '温度' },
        name: { type: 'string' },
      },
      required: ['t'],
    });
  });

  it('default 按类型解析：number/boolean/object/array', () => {
    const schema = rowsToSchema([
      row({ name: 'n', type: 'number', default: '35' }),
      row({ name: 'b', type: 'boolean', default: 'true' }),
      row({ name: 'o', type: 'object', default: '{"a":1}' }),
      row({ name: 'arr', type: 'array', default: '[1,2]' }),
      row({ name: 's', type: 'string', default: 'hi' }),
    ]);
    const props = schema!.properties as Record<string, { default: unknown }>;
    expect(props.n.default).toBe(35);
    expect(props.b.default).toBe(true);
    expect(props.o.default).toEqual({ a: 1 });
    expect(props.arr.default).toEqual([1, 2]);
    expect(props.s.default).toBe('hi');
  });

  it('default 解析失败回退字符串原值', () => {
    const schema = rowsToSchema([row({ name: 'o', type: 'object', default: '{bad json' })]);
    const props = schema!.properties as Record<string, { default: unknown }>;
    expect(props.o.default).toBe('{bad json');
  });

  it('空 default 不写入 schema', () => {
    const schema = rowsToSchema([row({ name: 'x', type: 'string', default: '  ' })]);
    const props = schema!.properties as Record<string, Record<string, unknown>>;
    expect('default' in props.x).toBe(false);
  });

  it('无必填时不输出 required 键', () => {
    const schema = rowsToSchema([row({ name: 'x' })]);
    expect('required' in schema!).toBe(false);
  });
});

describe('schemaToRows', () => {
  it('读 properties + required，缺省类型归 string', () => {
    const rows = schemaToRows({
      type: 'object',
      properties: {
        t: { type: 'number', description: '温度', default: 35 },
        name: { description: '无名类型' },
      },
      required: ['t'],
    });
    expect(rows).toHaveLength(2);
    const t = rows.find((r) => r.name === 't')!;
    expect(t.type).toBe('number');
    expect(t.required).toBe(true);
    expect(t.description).toBe('温度');
    expect(t.default).toBe('35');
    const name = rows.find((r) => r.name === 'name')!;
    expect(name.type).toBe('string');
    expect(name.required).toBe(false);
  });

  it('非 object / 无 properties / undefined → []', () => {
    expect(schemaToRows(undefined)).toEqual([]);
    expect(schemaToRows({ type: 'object' })).toEqual([]);
    expect(schemaToRows({ properties: 'nope' as unknown as Record<string, unknown> })).toEqual([]);
  });

  it('往返一致（schema → rows → schema）', () => {
    const original = {
      type: 'object',
      properties: {
        t: { type: 'number', description: '温度', default: 35 },
        ok: { type: 'boolean', default: true },
      },
      required: ['t'],
    };
    const round = rowsToSchema(schemaToRows(original));
    expect(round).toEqual(original);
  });
});

describe('stripJsonComments', () => {
  it('剥除行注释与块注释', () => {
    const src = `{
  // 行注释
  "a": 1, /* 块注释 */
  "b": 2
}`;
    expect(JSON.parse(stripJsonComments(src))).toEqual({ a: 1, b: 2 });
  });

  it('字符串内的 // 不误删（如 URL）', () => {
    const src = `{ "url": "http://example.com/a", "p": "x/*not*/y" }`;
    expect(JSON.parse(stripJsonComments(src))).toEqual({
      url: 'http://example.com/a',
      p: 'x/*not*/y',
    });
  });

  it('字符串内的转义引号不中断字符串状态', () => {
    const src = `{ "q": "he said \\"hi\\" // tail" } // real comment`;
    expect(JSON.parse(stripJsonComments(src))).toEqual({ q: 'he said "hi" // tail' });
  });

  it('注释独占一行时去掉整行（保留换行）', () => {
    const src = `{\n// c1\n"a":1\n}`;
    expect(stripJsonComments(src)).toBe('{\n\n"a":1\n}');
  });
});

describe('parseJsonc', () => {
  it('解析带注释 JSON', () => {
    expect(parseJsonc('{ "a": 1 } // ok')).toEqual({ a: 1 });
  });

  it('非法 JSON 抛错', () => {
    expect(() => parseJsonc('{ bad }')).toThrow();
  });
});
