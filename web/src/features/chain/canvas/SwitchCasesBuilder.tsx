// Switch 分支可视化构建器：参考 IF/ELIF/ELSE 设计，规则行（左值/运算符/右值）⇄ expr-lang 表达式。
// 受控组件：value 为 [{case, then}]，onChange 回传同构数组（写回 RuleGo switch 的 configuration.cases）。
import { useEffect, useMemo, useRef, useState } from 'react';
import { Button, Input, Segmented, Select, Tooltip } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';

import {
  RULE_OPS,
  emptyRule,
  exprToRules,
  rulesToExpr,
  type Combinator,
  type CondRule,
} from './switchExpr';

export interface SwitchCaseItem {
  case: string;
  then: string;
}

export interface SwitchCasesBuilderProps {
  value?: SwitchCaseItem[];
  onChange?: (v: SwitchCaseItem[]) => void;
}

// 分支内部编辑态：把 case 表达式解析为简单规则行；解析不了 → 高级模式保留原文。
// 每个条件的且/或连接符存在规则行自身（CondRule.join），不再用分支级 combinator——
// 这样同一分支内可以混用 且/或，改一个条件不影响其它。
interface BranchState {
  mode: 'simple' | 'advanced';
  rules: CondRule[];
  expr: string; // 高级模式原文，或简单模式最近一次生成值（切高级时作起点）
}

function toBranchState(expr: string): BranchState {
  const parsed = exprToRules(expr);
  if (parsed && parsed.rules.length > 0) {
    return { mode: 'simple', rules: parsed.rules, expr };
  }
  // 空表达式 → 给一条空规则行进简单模式；非空但解析不了 → 高级模式
  if (expr.trim() === '') {
    return { mode: 'simple', rules: [emptyRule()], expr: '' };
  }
  return { mode: 'advanced', rules: [emptyRule()], expr };
}

// 由受控 value 重建全部分支编辑态（仅用于外部变化：切换节点 / 重新加载 DSL）。
function toStates(items: SwitchCaseItem[]): Record<number, BranchState> {
  const next: Record<number, BranchState> = {};
  items.forEach((it, i) => {
    next[i] = toBranchState(it.case ?? '');
  });
  return next;
}

// value 的结构化指纹：用于区分「本组件 onChange 回环」与「外部重置」。
function fingerprint(items: SwitchCaseItem[] | undefined): string {
  return JSON.stringify(items ?? []);
}

export default function SwitchCasesBuilder({ value, onChange }: SwitchCasesBuilderProps) {
  const items = useMemo(() => value ?? [], [value]);
  // 每个分支的结构化编辑态（按索引记忆，新增/删除时对齐）。
  // 这是条件运算符/连接符的「事实源」：expr 字符串本身不含运算符结构
  // （每个比较都生成 left OP right 同一形态），受控回环时若从字符串重解析会把运算符
  // 全部塌缩回默认 ==。因此 states 一旦建立不回读 value，除非外部整体重置。
  const [states, setStates] = useState<Record<number, BranchState>>(() => toStates(items));
  // 记录最近一次由本组件 onChange 发出的 value 指纹，识别「自身回环」，
  // 避免在受控回环中重解析表达式、塌缩每个条件的运算符（仿 InputSchemaField 的 lastEmittedRef）。
  const lastEmittedRef = useRef<string | null>(null);

  // 外部 value 变化（切换节点 / 重新加载 DSL / 非本组件修改）时整体重建编辑态。
  // 若新值正是本组件刚发出的（受控回环），跳过重建以保留结构化规则行。
  useEffect(() => {
    const incoming = fingerprint(value);
    if (lastEmittedRef.current !== null && incoming === lastEmittedRef.current) {
      lastEmittedRef.current = null; // 自身回环：不重建，保住每个条件的运算符/连接符
      return;
    }
    lastEmittedRef.current = null;
    setStates(toStates(value ?? []));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fingerprint(value)]);

  const stateOf = (i: number): BranchState => states[i] ?? toBranchState(items[i]?.case ?? '');

  const emit = (next: SwitchCaseItem[]) => {
    lastEmittedRef.current = fingerprint(next);
    onChange?.(next);
  };

  const setBranch = (i: number, st: BranchState, exprOverride?: string) => {
    setStates((s) => ({ ...s, [i]: st }));
    const expr = exprOverride ?? (st.mode === 'simple' ? rulesToExpr(st.rules) : st.expr);
    emit(items.map((it, idx) => (idx === i ? { ...it, case: expr } : it)));
  };

  const setThen = (i: number, then: string) =>
    emit(items.map((it, idx) => (idx === i ? { ...it, then } : it)));

  const addBranch = () => {
    const nextThen = `Case${items.length + 1}`;
    setStates((s) => ({ ...s, [items.length]: { mode: 'simple', rules: [emptyRule()], expr: '' } }));
    emit([...items, { case: '', then: nextThen }]);
  };

  const removeBranch = (i: number) => {
    setStates((s) => {
      const copy: Record<number, BranchState> = {};
      // 删除点之后的分支编辑态前移一位，保持与新 items 索引对齐
      Object.entries(s).forEach(([k, v]) => {
        const idx = Number(k);
        if (idx === i) return;
        copy[idx > i ? idx - 1 : idx] = v;
      });
      return copy;
    });
    emit(items.filter((_, idx) => idx !== i));
  };

  const toggleMode = (i: number, mode: 'simple' | 'advanced') => {
    const st = stateOf(i);
    if (mode === st.mode) return;
    if (mode === 'advanced') {
      // 简单 → 高级：以当前生成的表达式为起点
      const expr = rulesToExpr(st.rules);
      setBranch(i, { ...st, mode: 'advanced', expr }, expr);
    } else {
      // 高级 → 简单：尝试解析；解析不了则给空规则行（保留 expr 供随时切回）
      const parsed = exprToRules(st.expr);
      const rules = parsed && parsed.rules.length > 0 ? parsed.rules : [emptyRule()];
      setBranch(i, { ...st, mode: 'simple', rules });
    }
  };

  const updateRule = (i: number, key: string, patch: Partial<CondRule>) => {
    const st = stateOf(i);
    const rules = st.rules.map((r) => (r.key === key ? { ...r, ...patch } : r));
    setBranch(i, { ...st, rules });
  };
  const addRule = (i: number) => {
    const st = stateOf(i);
    setBranch(i, { ...st, rules: [...st.rules, { ...emptyRule(), join: '&&' }] });
  };
  const removeRule = (i: number, key: string) => {
    const st = stateOf(i);
    const rules = st.rules.filter((r) => r.key !== key);
    setBranch(i, { ...st, rules: rules.length > 0 ? rules : [emptyRule()] });
  };

  return (
    <div>
      {items.map((item, i) => {
        const st = stateOf(i);
        const branchLabel = i === 0 ? 'IF' : `ELIF ${i}`;
        return (
          <div key={i} className="bf-switch-branch">
            <div className="bf-switch-branch-head">
              <span className="bf-switch-tag">{branchLabel}</span>
              <Input
                size="small"
                className="bf-switch-then"
                value={item.then}
                placeholder="分支名（路由连接类型）"
                onChange={(e) => setThen(i, e.target.value)}
              />
              <Segmented
                size="small"
                value={st.mode}
                onChange={(m) => toggleMode(i, m as 'simple' | 'advanced')}
                options={[
                  { value: 'simple', label: '构建' },
                  { value: 'advanced', label: '表达式' },
                ]}
              />
              <Tooltip title="删除该分支">
                <Button danger type="text" size="small" icon={<DeleteOutlined />} onClick={() => removeBranch(i)} />
              </Tooltip>
            </div>

            {st.mode === 'simple' ? (
              <div className="bf-switch-rules">
                {st.rules.map((r, ri) => {
                  const needsValue = RULE_OPS.find((o) => o.value === r.op)?.needsValue ?? true;
                  return (
                    /* 两行：行1=连接符+运算符(全宽,始终可见)+删除；行2=左值+右值 */
                    <div key={r.key} className="bf-switch-rule">
                      <div className="bf-switch-rule-top">
                        {ri === 0 ? (
                          <span className="bf-switch-join bf-switch-join-first">当</span>
                        ) : (
                          <Select
                            size="small"
                            className="bf-switch-join"
                            value={r.join ?? '&&'}
                            onChange={(c) => updateRule(i, r.key, { join: c as Combinator })}
                            options={[
                              { value: '&&', label: '且' },
                              { value: '||', label: '或' },
                            ]}
                          />
                        )}
                        <Select
                          size="small"
                          className="bf-switch-op"
                          value={r.op}
                          onChange={(op) => updateRule(i, r.key, { op })}
                          options={RULE_OPS.map((o) => ({ value: o.value, label: o.label }))}
                        />
                        <Button
                          danger
                          type="text"
                          size="small"
                          icon={<DeleteOutlined />}
                          aria-label="删除条件"
                          onClick={() => removeRule(i, r.key)}
                        />
                      </div>
                      <div className="bf-switch-rule-io">
                        <Input
                          size="small"
                          className="bf-switch-left"
                          value={r.left}
                          placeholder="左值，如 msg.temperature"
                          onChange={(e) => updateRule(i, r.key, { left: e.target.value })}
                        />
                        {needsValue ? (
                          <Input
                            size="small"
                            className="bf-switch-right"
                            value={r.right}
                            placeholder="右值"
                            onChange={(e) => updateRule(i, r.key, { right: e.target.value })}
                          />
                        ) : null}
                      </div>
                    </div>
                  );
                })}
                <Button type="dashed" size="small" icon={<PlusOutlined />} onClick={() => addRule(i)}>
                  添加条件
                </Button>
              </div>
            ) : (
              <Input.TextArea
                rows={2}
                value={st.expr}
                placeholder='布尔表达式，如 msg.temperature > 50 && msg.name == "a"；变量：msg / metadata / type'
                style={{ fontFamily: 'monospace', fontSize: 12 }}
                onChange={(e) => setBranch(i, { ...stateOf(i), expr: e.target.value }, e.target.value)}
              />
            )}
          </div>
        );
      })}

      <div className="bf-switch-else">
        <span className="bf-switch-tag bf-switch-tag-else">ELSE</span>
        <span className="bf-switch-else-desc">以上条件都不满足时走 Default 分支</span>
      </div>

      <Button type="dashed" block icon={<PlusOutlined />} onClick={addBranch}>
        添加 ELIF 分支
      </Button>
    </div>
  );
}
