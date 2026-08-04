// Switch 分支可视化构建器：参考 IF/ELIF/ELSE 设计，规则行（左值/运算符/右值）⇄ expr-lang 表达式。
// 受控组件：value 为 [{case, then}]，onChange 回传同构数组（写回 RuleGo switch 的 configuration.cases）。
import { useEffect, useMemo, useRef, useState } from 'react';
import { Button, Input, Select, Space, Tooltip } from 'antd';
import { CloseOutlined, MenuOutlined, PlusOutlined } from '@ant-design/icons';

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

// 按 OR 把扁平规则行切成可视分组（截图布局：一组 = 左侧竖向导引条 + 组内若干行，
// 组间为 OR，组内为 AND）。仅影响渲染；底层仍是带 per-rule join 的扁平数组。
interface OrGroup {
  rules: CondRule[]; // 组内规则（join 相对组内前一条；首条忽略）
  join: Combinator; // 组内连接符（仅 AND 组可编辑）
  isOrGroup: boolean; // true= OR 组（单一条件，「添加 OR 条件」再挂一条）
}

function groupByOr(rules: CondRule[]): OrGroup[] {
  const out: OrGroup[] = [];
  let cur: CondRule[] = [];
  let curJoin: Combinator = '&&';
  rules.forEach((r, i) => {
    if (i === 0) {
      cur = [r];
      curJoin = '&&';
      return;
    }
    const j = r.join ?? '&&';
    if (j === '&&') {
      cur.push(r); // 并入当前 AND 组
      return;
    }
    // 遇到 ||：结束当前组，开启一个新的 OR 组（每组仅一条）
    out.push({ rules: cur, join: curJoin, isOrGroup: false });
    cur = [r];
    curJoin = '||';
  });
  if (cur.length > 0) out.push({ rules: cur, join: curJoin, isOrGroup: out.length > 0 });
  return out;
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

  // 高级模式 ⇄ 构建模式：头部小图标循环切换（截图头部仅 IF + 添加OR + 关闭）。
  const cycleMode = (i: number) => {
    const st = stateOf(i);
    toggleMode(i, st.mode === 'simple' ? 'advanced' : 'simple');
  };

  const updateRule = (i: number, key: string, patch: Partial<CondRule>) => {
    const st = stateOf(i);
    const rules = st.rules.map((r) => (r.key === key ? { ...r, ...patch } : r));
    setBranch(i, { ...st, rules });
  };
  // 组内「添加条件」→ join '&&'（并入当前 AND 组）；
  // 头部「添加 OR 条件」→ join '||'（另起 OR 组）。两者只是新行的 join 不同。
  const addRule = (i: number, join: Combinator = '&&') => {
    const st = stateOf(i);
    setBranch(i, { ...st, rules: [...st.rules, { ...emptyRule(), join }] });
  };
  const removeRule = (i: number, key: string) => {
    const st = stateOf(i);
    const rules = st.rules.filter((r) => r.key !== key);
    setBranch(i, { ...st, rules: rules.length > 0 ? rules : [emptyRule()] });
  };

  // 渲染一条规则行的「运算符」下拉（内嵌在左值输入框后，类名 .bf-switch-op 供测试定位）。
  const renderOp = (i: number, r: CondRule, width: number) => (
    <Select
      size="small"
      variant="borderless"
      className="bf-switch-op"
      style={{ width, flex: 'none' }}
      popupMatchSelectWidth={false}
      value={r.op}
      onChange={(op) => updateRule(i, r.key, { op })}
      options={RULE_OPS.map((o) => ({ value: o.value, label: o.label }))}
    />
  );

  // 一组（AND 组 或 OR 组）的渲染：左侧竖向导引条 + 组内规则行。
  const renderGroup = (i: number, group: OrGroup) => {
    const canEditJoin = group.rules.length > 1; // 组内 >1 条才可切换 AND/OR
    return (
      <div className="bf-switch-cond">
        {/* 左侧竖向导引条：标记本组组内连接符（AND/OR） */}
        <div className="bf-switch-rail">
          {canEditJoin ? (
            <Select
              size="small"
              variant="borderless"
              className="bf-switch-join"
              popupMatchSelectWidth={false}
              value={group.join}
              onChange={(c) => {
                const j = c as Combinator;
                group.rules.forEach((r, gi) => {
                  if (gi > 0) updateRule(i, r.key, { join: j });
                });
              }}
              options={[
                { value: '&&', label: 'AND' },
                { value: '||', label: 'OR' },
              ]}
            />
          ) : (
            <span className="bf-switch-join bf-switch-join-static">
              {group.isOrGroup ? 'OR' : 'AND'}
            </span>
          )}
        </div>

        {/* 组内规则行：单行 = [左值(内嵌运算符) | 右值 | 删除] */}
        <div className="bf-switch-rules">
          {group.rules.map((r) => {
            const needsValue = RULE_OPS.find((o) => o.value === r.op)?.needsValue ?? true;
            return (
              <div key={r.key} className="bf-switch-rule">
                <div className="bf-switch-rule-io">
                  {/* 左值 + 运算符（Space.Compact 使两者融为一体，运算符贴右缘） */}
                  <Space.Compact className="bf-switch-left">
                    <Input
                      size="small"
                      value={r.left}
                      placeholder="左值，如 msg.temperature"
                      onChange={(e) => updateRule(i, r.key, { left: e.target.value })}
                    />
                    {renderOp(i, r, needsValue ? 92 : 130)}
                  </Space.Compact>
                  {needsValue ? (
                    <Input
                      size="small"
                      className="bf-switch-right"
                      value={r.right}
                      placeholder="右值"
                      onChange={(e) => updateRule(i, r.key, { right: e.target.value })}
                    />
                  ) : null}
                  <Button
                    danger
                    type="text"
                    size="small"
                    icon={<CloseOutlined />}
                    aria-label="删除条件"
                    onClick={() => removeRule(i, r.key)}
                  />
                </div>
              </div>
            );
          })}
          <button type="button" className="bf-switch-add-cond" onClick={() => addRule(i)}>
            <PlusOutlined /> 添加条件
          </button>
        </div>
      </div>
    );
  };

  return (
    <div>
      {items.map((item, i) => {
        const st = stateOf(i);
        const branchLabel = i === 0 ? 'IF' : `ELIF ${i}`;
        return (
          <div key={i} className="bf-switch-branch">
            {/* 分支头：IF 标签 + 分支名 + 添加 OR 条件 + 模式切换 + 关闭 */}
            <div className="bf-switch-branch-head">
              <span className="bf-switch-tag">{branchLabel}</span>
              <Input
                size="small"
                className="bf-switch-then"
                value={item.then}
                placeholder="分支名（路由连接类型）"
                onChange={(e) => setThen(i, e.target.value)}
              />
              {st.mode === 'simple' ? (
                <button
                  type="button"
                  className="bf-switch-add-or"
                  onClick={() => addRule(i, '||')}
                >
                  添加 OR 条件
                </button>
              ) : null}
              <Tooltip title={st.mode === 'simple' ? '切换为表达式编辑' : '切换为可视化构建'}>
                <Button
                  type="text"
                  size="small"
                  icon={<MenuOutlined />}
                  aria-label="切换编辑模式"
                  onClick={() => cycleMode(i)}
                />
              </Tooltip>
              <Tooltip title="删除该分支">
                <Button
                  danger
                  type="text"
                  size="small"
                  icon={<CloseOutlined />}
                  aria-label="删除分支"
                  onClick={() => removeBranch(i)}
                />
              </Tooltip>
            </div>

            {st.mode === 'simple' ? (
              <div className="bf-switch-groups">
                {groupByOr(st.rules).map((g, gi) => (
                  <div key={g.rules[0]?.key ?? gi}>{renderGroup(i, g)}</div>
                ))}
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
        <span className="bf-switch-else-desc">ELSE 用于定义当 if 条件不满足时执行的逻辑。</span>
      </div>

      <Button type="text" className="bf-switch-add-branch" icon={<PlusOutlined />} onClick={addBranch}>
        ELIF
      </Button>
    </div>
  );
}
