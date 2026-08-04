import { describe, it, expect, beforeAll } from 'vitest';
import { useState } from 'react';
import { render, fireEvent } from '@testing-library/react';

import type { ComponentFormField } from '@/api/component';
import StructArrayField from './StructArrayField';

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

// cacheSet.items 的子字段 schema（模拟后端反射产物）。
const CACHE_ITEM_FIELDS: ComponentFormField[] = [
  { name: 'level', type: 'string' },
  { name: 'key', type: 'string' },
  { name: 'value', type: 'string' },
  { name: 'ttl', type: 'string' },
];

function Harness({ initial }: { initial?: Array<Record<string, unknown>> }) {
  const [v, setV] = useState<unknown>(initial);
  return (
    <StructArrayField
      subFields={CACHE_ITEM_FIELDS}
      ruleType="cacheSet"
      value={v}
      onChange={setV}
    />
  );
}

describe('StructArrayField', () => {
  it('按初始值渲染行，编辑后向上输出对象数组', () => {
    const { container, getAllByPlaceholderText } = render(
      <Harness initial={[{ level: 'chain', key: 'a', value: '1', ttl: '10s' }]} />,
    );
    const keyInputs = getAllByPlaceholderText('键');
    expect(keyInputs).toHaveLength(1);
    expect((keyInputs[0] as HTMLInputElement).value).toBe('a');
    expect(container.querySelectorAll('.ant-btn')).not.toHaveLength(0);
  });

  it('添加一行：level 默认 chain，新增空行可编辑', () => {
    const emitted: unknown[] = [];
    function Controlled() {
      const [v, setV] = useState<unknown>([]);
      return (
        <StructArrayField
          subFields={CACHE_ITEM_FIELDS}
          ruleType="cacheSet"
          value={v}
          onChange={(nv) => {
            emitted.push(nv);
            setV(nv);
          }}
        />
      );
    }
    const { getByRole, getByPlaceholderText } = render(<Controlled />);
    fireEvent.click(getByRole('button', { name: /添加一行/ }));
    const keyInput = getByPlaceholderText('键') as HTMLInputElement;
    fireEvent.change(keyInput, { target: { value: 'myKey' } });
    const last = emitted[emitted.length - 1] as Array<Record<string, unknown>>;
    expect(last).toHaveLength(1);
    expect(last[0].key).toBe('myKey');
    expect(last[0].level).toBe('chain');
  });

  it('删除一行：从输出中移除对应对象', () => {
    function Controlled() {
      const [v, setV] = useState<unknown>([
        { level: 'chain', key: 'a', value: '1' },
        { level: 'global', key: 'b', value: '2' },
      ]);
      return (
        <StructArrayField
          subFields={CACHE_ITEM_FIELDS}
          ruleType="cacheSet"
          value={v}
          onChange={setV}
        />
      );
    }
    const { getAllByLabelText, queryByDisplayValue } = render(<Controlled />);
    const delButtons = getAllByLabelText('删除该行');
    expect(delButtons).toHaveLength(2);
    fireEvent.click(delButtons[0]);
    expect(queryByDisplayValue('a')).toBeNull();
    expect(queryByDisplayValue('b')).not.toBeNull();
  });

  it('非数组/非法值渲染为空列表', () => {
    const { queryByPlaceholderText } = render(
      <Harness initial={undefined} />,
    );
    expect(queryByPlaceholderText('键')).toBeNull();
  });
});
