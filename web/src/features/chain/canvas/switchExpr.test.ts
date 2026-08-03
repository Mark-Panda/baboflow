import { describe, expect, it } from 'vitest';

import {
  emptyRule,
  exprToRules,
  ruleToExpr,
  rulesToExpr,
  type CondRule,
} from './switchExpr';

function rule(p: Partial<CondRule>): CondRule {
  return { key: p.key ?? `k_${Math.random()}`, left: 'msg.x', op: '==', ...p } as CondRule;
}

describe('ruleToExpr', () => {
  it('数字右值不加引号', () => {
    expect(ruleToExpr(rule({ left: 'msg.temperature', op: '>', right: '50' }))).toBe('msg.temperature > 50');
  });
  it('字符串右值加双引号', () => {
    expect(ruleToExpr(rule({ left: 'msg.name', op: '==', right: 'test1' }))).toBe('msg.name == "test1"');
  });
  it('布尔/nil 原样', () => {
    expect(ruleToExpr(rule({ left: 'msg.ok', op: '==', right: 'true' }))).toBe('msg.ok == true');
  });
  it('字符串右值转义引号', () => {
    expect(ruleToExpr(rule({ left: 'msg.s', op: '==', right: 'a"b' }))).toBe('msg.s == "a\\"b"');
  });
  it('contains / not contains', () => {
    expect(ruleToExpr(rule({ left: 'msg.tag', op: 'contains', right: 'ab' }))).toBe('contains(msg.tag, "ab")');
    expect(ruleToExpr(rule({ left: 'msg.tag', op: 'not contains', right: 'ab' }))).toBe('!contains(msg.tag, "ab")');
  });
  it('startsWith / endsWith / matches', () => {
    expect(ruleToExpr(rule({ left: 'msg.p', op: 'startsWith', right: 'http' }))).toBe('startsWith(msg.p, "http")');
    expect(ruleToExpr(rule({ left: 'msg.p', op: 'endsWith', right: '.png' }))).toBe('endsWith(msg.p, ".png")');
    expect(ruleToExpr(rule({ left: 'msg.p', op: 'matches', right: '^a' }))).toBe('matches(msg.p, "^a")');
  });
  it('empty / not empty', () => {
    expect(ruleToExpr(rule({ left: 'msg.v', op: 'empty' }))).toBe('(msg.v == nil || msg.v == "")');
    expect(ruleToExpr(rule({ left: 'msg.v', op: 'not empty' }))).toBe('(msg.v != nil && msg.v != "")');
  });
});

describe('rulesToExpr', () => {
  it('单条不加外层括号', () => {
    expect(rulesToExpr([rule({ left: 'msg.t', op: '>', right: '50' })])).toBe('msg.t > 50');
  });
  it('多条默认按 && 连接且每条加括号', () => {
    const out = rulesToExpr([
      rule({ left: 'msg.temperature', op: '>', right: '30' }),
      rule({ left: 'msg.humidity', op: '>', right: '20' }),
    ]);
    expect(out).toBe('(msg.temperature > 30) && (msg.humidity > 20)');
  });
  it('按规则行 join 用 || 连接', () => {
    const out = rulesToExpr([
      rule({ left: 'msg.a', op: '==', right: '1' }),
      rule({ left: 'msg.b', op: '==', right: '2', join: '||' }),
    ]);
    expect(out).toBe('(msg.a == 1) || (msg.b == 2)');
  });
  it('同一分支混用 且/或：A 且 B 或 C（&& 段整体加括号保证优先级）', () => {
    const out = rulesToExpr([
      rule({ left: 'msg.a', op: '>', right: '1' }),
      rule({ left: 'msg.b', op: '>', right: '2', join: '&&' }),
      rule({ left: 'msg.c', op: '>', right: '3', join: '||' }),
    ]);
    // 语义 = (A && B) || C
    expect(out).toBe('((msg.a > 1) && (msg.b > 2)) || (msg.c > 3)');
  });
  it('混用：A 或 B 且 C → (A) || ((B) && (C))', () => {
    const out = rulesToExpr([
      rule({ left: 'msg.a', op: '>', right: '1' }),
      rule({ left: 'msg.b', op: '>', right: '2', join: '||' }),
      rule({ left: 'msg.c', op: '>', right: '3', join: '&&' }),
    ]);
    expect(out).toBe('(msg.a > 1) || ((msg.b > 2) && (msg.c > 3))');
  });
  it('空左值行被忽略；全空返回空串', () => {
    expect(rulesToExpr([rule({ left: '  ', right: '1' })])).toBe('');
  });
});

describe('exprToRules', () => {
  it('解析单个比较（RuleGo 真实例子）', () => {
    expect(exprToRules('msg.temperature > 50')?.rules).toEqual([
      { key: expect.any(String), left: 'msg.temperature', op: '>', right: '50', join: '&&' },
    ]);
    expect(exprToRules('msg.name == "test1"')?.rules[0]).toMatchObject({ left: 'msg.name', op: '==', right: 'test1' });
    expect(exprToRules('metadata.productType == "test"')?.rules[0]).toMatchObject({
      left: 'metadata.productType',
      op: '==',
      right: 'test',
    });
  });

  it('解析 >= <= !=', () => {
    expect(exprToRules('msg.a >= 5')?.rules[0].op).toBe('>=');
    expect(exprToRules('msg.a <= 5')?.rules[0].op).toBe('<=');
    expect(exprToRules('msg.a != 5')?.rules[0].op).toBe('!=');
  });

  it('解析 && 组合（RuleGo 真实例子）', () => {
    const out = exprToRules('msg.temperature > 30 && msg.humidity > 20');
    expect(out?.rules).toHaveLength(2);
    expect(out?.rules[1]).toMatchObject({ left: 'msg.humidity', op: '>', right: '20', join: '&&' });
  });

  it('解析带外层括号的组合（我们生成的形态）', () => {
    const out = exprToRules('(msg.temperature > 30) && (msg.humidity > 20)');
    expect(out?.rules).toHaveLength(2);
  });

  it('解析函数调用与取反', () => {
    expect(exprToRules('contains(msg.tag, "ab")')?.rules[0]).toMatchObject({ left: 'msg.tag', op: 'contains', right: 'ab' });
    expect(exprToRules('!contains(msg.tag, "ab")')?.rules[0]).toMatchObject({ left: 'msg.tag', op: 'not contains', right: 'ab' });
    expect(exprToRules('startsWith(msg.p, "http")')?.rules[0]).toMatchObject({ op: 'startsWith', right: 'http' });
  });

  it('解析 empty / not empty', () => {
    expect(exprToRules('(msg.v == nil || msg.v == "")')?.rules[0]).toMatchObject({ left: 'msg.v', op: 'empty' });
    expect(exprToRules('(msg.v != nil && msg.v != "")')?.rules[0]).toMatchObject({ left: 'msg.v', op: 'not empty' });
  });

  it('解析混用 && 与 ||：每个条件带自己的 join', () => {
    const out = exprToRules('msg.a > 1 && msg.b > 2 || msg.c > 3');
    expect(out?.rules).toHaveLength(3);
    expect(out?.rules.map((r) => r.join)).toEqual(['&&', '&&', '||']);
    expect(out?.rules.map((r) => r.left)).toEqual(['msg.a', 'msg.b', 'msg.c']);
  });

  it('无法解析的复杂表达式返回 null（走高级模式）', () => {
    expect(exprToRules('len(msg.items) > 0 && upper(msg.type) == "A"')).toBeNull();
  });

  it('往返一致（rules → expr → rules）', () => {
    const rules = [
      rule({ key: 'a', left: 'msg.temperature', op: '>', right: '30' }),
      rule({ key: 'b', left: 'msg.name', op: '==', right: 'test1', join: '&&' }),
    ];
    const expr = rulesToExpr(rules);
    const parsed = exprToRules(expr);
    expect(parsed?.rules.map((r) => [r.left, r.op, r.right, r.join])).toEqual([
      ['msg.temperature', '>', '30', '&&'],
      ['msg.name', '==', 'test1', '&&'],
    ]);
  });

  it('往返一致（混用 且/或）', () => {
    const rules = [
      rule({ key: 'a', left: 'msg.a', op: '>', right: '1' }),
      rule({ key: 'b', left: 'msg.b', op: '>', right: '2', join: '&&' }),
      rule({ key: 'c', left: 'msg.c', op: '>', right: '3', join: '||' }),
    ];
    const expr = rulesToExpr(rules);
    const parsed = exprToRules(expr);
    expect(parsed?.rules.map((r) => [r.left, r.op, r.right, r.join])).toEqual([
      ['msg.a', '>', '1', '&&'],
      ['msg.b', '>', '2', '&&'],
      ['msg.c', '>', '3', '||'],
    ]);
    // 再生成应与原表达式等价（括号形态可能不同，但解析回 rules 一致）
    expect(rulesToExpr(parsed!.rules)).toBe(expr);
  });
});

describe('emptyRule', () => {
  it('给默认 msg. 前缀', () => {
    expect(emptyRule().left).toBe('msg.');
  });
});
