import React from 'react';
import { describe, it, expect, beforeAll, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import SwitchCasesBuilder, { type SwitchCaseItem } from './SwitchCasesBuilder';

// jsdom 缺 matchMedia，antd 组件需要。
beforeAll(() => {
  window.matchMedia =
    window.matchMedia ||
    ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }));
});

// 受控回环：模拟 NodeConfigPanel 的 antd Form —— value/onChange 双向绑定。
// 每次 onChange 都把新值灌回 value（这正是之前塌缩运算符的回环路径）。
function Harness({
  initial,
  onSpy,
}: {
  initial: SwitchCaseItem[];
  onSpy: (v: SwitchCaseItem[]) => void;
}) {
  const [v, setV] = React.useState<SwitchCaseItem[]>(initial);
  return (
    <SwitchCasesBuilder
      value={v}
      onChange={(next) => {
        setV(next ?? []);
        onSpy(next ?? []);
      }}
    />
  );
}

const opSelects = () =>
  Array.from(window.document.querySelectorAll('.bf-switch-op')) as HTMLElement[];

async function pickOption(label: string) {
  // 下拉渲染到 body，取最后一个匹配项（当前展开的那个）
  const found = await screen.findAllByText(label);
  fireEvent.click(found[found.length - 1]);
}

describe('SwitchCasesBuilder 每个条件独立运算符（受控回环不塌缩）', () => {
  it('同一 IF 内两个条件可用不同运算符（> 与 <），回环后不重置', async () => {
    const spy = vi.fn();
    render(<Harness initial={[{ case: '', then: 'Case1' }]} onSpy={spy} />);

    // 添加第二个条件 → 两条规则行
    fireEvent.click(screen.getByRole('button', { name: /添加条件/ }));
    expect(opSelects().length).toBe(2);

    // 第一个条件运算符 → 大于
    fireEvent.mouseDown(opSelects()[0].querySelector('.ant-select-selector')!);
    await pickOption('大于');

    // 第二个条件运算符 → 小于
    fireEvent.mouseDown(opSelects()[1].querySelector('.ant-select-selector')!);
    await pickOption('小于');

    // 生成的表达式应同时含 > 和 <（两个条件各自独立的运算符）
    const emitted = spy.mock.calls[spy.mock.calls.length - 1][0] as SwitchCaseItem[];
    expect(emitted[0].case).toContain('>');
    expect(emitted[0].case).toContain('<');

    // 关键回归：受控回环（每次 onChange 都灌回 value）后，
    // 两个运算符选择器仍应显示「大于」「小于」，而不是塌缩回「等于」。
    const shown = opSelects().map((el) => el.textContent);
    expect(shown[0]).toContain('大于');
    expect(shown[1]).toContain('小于');
  });

  it('单行布局：运算符内嵌在左值输入框后，左/右值同一行', async () => {
    render(
      <Harness
        initial={[{ case: 'msg.temperature == 20 && msg.temperature < 50', then: 'Case1' }]}
        onSpy={() => {}}
      />
    );
    // 两个条件 → 两个 .bf-switch-rule，每个规则内一个 .bf-switch-op（内嵌在左值后）
    const rules = Array.from(window.document.querySelectorAll('.bf-switch-rule'));
    expect(rules.length).toBe(2);
    const ops = opSelects();
    expect(ops.length).toBe(2);
    rules.forEach((rule) => {
      // 左值输入框承载运算符（addonAfter 内嵌 .bf-switch-op）
      const left = rule.querySelector('.bf-switch-left');
      expect(left).not.toBeNull();
      expect(left!.querySelector('.bf-switch-op')).not.toBeNull();
      // 同一行 (bf-switch-rule-io) 内含左右值输入框
      expect(rule.querySelector('.bf-switch-rule-io .bf-switch-right')).not.toBeNull();
    });
    // 两个条件各自显示自己的运算符文本
    expect(ops[0].textContent).toContain('等于');
    expect(ops[1].textContent).toContain('小于');
  });

  it('多分支间运算符互不影响（分支0 用 >，分支1 用 <）', async () => {
    const spy = vi.fn();
    render(
      <Harness
        initial={[{ case: '', then: 'Case1' }, { case: '', then: 'Case2' }]}
        onSpy={spy}
      />
    );

    expect(opSelects().length).toBe(2);

    fireEvent.mouseDown(opSelects()[0].querySelector('.ant-select-selector')!);
    await pickOption('大于');

    fireEvent.mouseDown(opSelects()[1].querySelector('.ant-select-selector')!);
    await pickOption('小于');

    const shown = opSelects().map((el) => el.textContent);
    expect(shown[0]).toContain('大于');
    expect(shown[1]).toContain('小于');

    const emitted = spy.mock.calls[spy.mock.calls.length - 1][0] as SwitchCaseItem[];
    expect(emitted[0].case).toContain('>');
    expect(emitted[1].case).toContain('<');
  });

  it('外部重置（切换节点/重载 DSL）时按新 value 重建', async () => {
    const spy = vi.fn();
    const { rerender } = render(
      <SwitchCasesBuilder
        value={[{ case: 'msg.aa > 1', then: 'Case1' }]}
        onChange={spy}
      />
    );
    // 初始：解析出 >
    expect(opSelects()[0].textContent).toContain('大于');

    // 外部把 value 换成另一个表达式（含 <）→ 应重建为 小于
    rerender(
      <SwitchCasesBuilder
        value={[{ case: 'msg.aa < 9', then: 'Case1' }]}
        onChange={spy}
      />
    );
    expect(opSelects()[0].textContent).toContain('小于');
  });

  it('「添加 OR 条件」新增的行 join 为 ||（独立 OR 组，表达式含 ||）', () => {
    const spy = vi.fn();
    render(<Harness initial={[{ case: '', then: 'Case1' }]} onSpy={spy} />);

    // 初始 1 条 → 点「添加 OR 条件」→ 应变 2 条
    expect(opSelects().length).toBe(1);
    fireEvent.click(screen.getByRole('button', { name: '添加 OR 条件' }));
    expect(opSelects().length).toBe(2);

    // 新行 join 必须是 ||，生成的表达式含 ||（否则被并进同一 AND 组、视觉无变化）
    const emitted = spy.mock.calls[spy.mock.calls.length - 1][0] as SwitchCaseItem[];
    expect(emitted[0].case).toContain('||');
  });

  it('「添加条件」新增的行 join 为 &&（并入当前 AND 组，表达式含 &&）', () => {
    const spy = vi.fn();
    render(<Harness initial={[{ case: '', then: 'Case1' }]} onSpy={spy} />);

    expect(opSelects().length).toBe(1);
    fireEvent.click(screen.getByRole('button', { name: /(?<!OR )添加条件$/ }));
    expect(opSelects().length).toBe(2);

    const emitted = spy.mock.calls[spy.mock.calls.length - 1][0] as SwitchCaseItem[];
    expect(emitted[0].case).toContain('&&');
    expect(emitted[0].case).not.toContain('||');
  });
});
