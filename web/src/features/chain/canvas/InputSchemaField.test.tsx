import { describe, it, expect, beforeAll } from 'vitest';
import { useState } from 'react';
import { render, fireEvent } from '@testing-library/react';

import InputSchemaField from './InputSchemaField';

// jsdom 缺 matchMedia，antd 响应式栅格/表格需要。
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

// 模拟 antd Form 的受控回环：子组件 onChange → 父级 setState → 以新 value 重新渲染。
// 这正是打断中文输入法（IME）composition 的写回路径。
function Harness({ initial }: { initial?: Record<string, unknown> }) {
  const [v, setV] = useState<Record<string, unknown> | undefined>(initial);
  return <InputSchemaField value={v} onChange={setV} />;
}

describe('InputSchemaField 受控回环（IME 连续输入回归）', () => {
  it('打字过程中输入框不被重置（value 连续累积）', () => {
    const initial = {
      type: 'object',
      properties: { t: { type: 'number' } },
      required: ['t'],
    };
    const { getByPlaceholderText } = render(<Harness initial={initial} />);

    // 「参数名」列输入框（placeholder 在 InputSchemaField 中固定为"如 t"）
    const nameInput = getByPlaceholderText('如 t') as HTMLInputElement;
    expect(nameInput.value).toBe('t');

    // 逐字符输入，模拟连续键入（含 IME 上屏后的多次 change）。
    // 每次 change 都会触发 commit→onChange→父级写回 value；
    // 修复前：useEffect 重置 rows → Table 以新 key 重建单元格 → 输入被清空/失焦。
    fireEvent.change(nameInput, { target: { value: 'te' } });
    expect((getByPlaceholderText('如 t') as HTMLInputElement).value).toBe('te');

    fireEvent.change(getByPlaceholderText('如 t'), { target: { value: 'tem' } });
    expect((getByPlaceholderText('如 t') as HTMLInputElement).value).toBe('tem');

    fireEvent.change(getByPlaceholderText('如 t'), { target: { value: 'temp' } });
    expect((getByPlaceholderText('如 t') as HTMLInputElement).value).toBe('temp');
  });

  it('外部值真正变化时仍会重置内部行（不吞掉合法更新）', () => {
    const initial = {
      type: 'object',
      properties: { t: { type: 'number' } },
    };
    const { rerender, getByPlaceholderText, queryByPlaceholderText } = render(
      <InputSchemaField value={initial} onChange={() => {}} />,
    );
    expect(getByPlaceholderText('如 t')).toBeTruthy();

    // 换成完全不同的 schema（如切换链）：内部应同步为新行，而不是停留在旧行。
    const next = {
      type: 'object',
      properties: { name: { type: 'string' } },
    };
    rerender(<InputSchemaField value={next} onChange={() => {}} />);
    // 旧的 "t" 行应消失，新行 name 应出现（仍只有一个参数名输入框）。
    const inputs = queryByPlaceholderText('如 t') as HTMLInputElement | null;
    expect(inputs).toBeTruthy();
    expect((inputs as HTMLInputElement).value).toBe('name');
  });
});
