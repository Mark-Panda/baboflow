import { describe, it, expect, beforeAll } from 'vitest';
import { useState } from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { Form, Input } from 'antd';

import InputSchemaField from './InputSchemaField';

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

// 复刻 ChainSettingsForm 的关键结构：名称/描述用本地 state 自管受控（modal.confirm 一次性渲染、
// 父组件不重渲染，若直接用 props 受控会被旧值"顶回"导致无法编辑）。这里单独测这个契约。
function SettingsFormLike({
  name,
  description,
  inputSchema,
  onChange,
}: {
  name: string;
  description: string;
  inputSchema?: Record<string, unknown>;
  onChange: (p: { name?: string; description?: string; inputSchema?: Record<string, unknown> }) => void;
}) {
  const [localName, setLocalName] = useState(name);
  const [localDesc, setLocalDesc] = useState(description);
  return (
    <Form layout="vertical" size="small">
      <Form.Item label="规则链名称" required>
        <Input
          value={localName}
          placeholder="规则链名称"
          onChange={(e) => {
            setLocalName(e.target.value);
            onChange({ name: e.target.value });
          }}
        />
      </Form.Item>
      <Form.Item label="规则链描述">
        <Input.TextArea
          rows={3}
          value={localDesc}
          onChange={(e) => {
            setLocalDesc(e.target.value);
            onChange({ description: e.target.value });
          }}
        />
      </Form.Item>
      <Form.Item label="入参格式">
        <InputSchemaField value={inputSchema} onChange={(v) => onChange({ inputSchema: v })} />
      </Form.Item>
    </Form>
  );
}

describe('保存草稿弹窗 · 键设置表单可编辑', () => {
  it('规则链名称可输入修改（本地受控），并通过 onChange 同步最新值', () => {
    const patches: Array<{ name?: string }> = [];
    render(
      <SettingsFormLike
        name="订单处理链"
        description=""
        onChange={(p) => patches.push(p)}
      />,
    );
    const nameInput = screen.getByPlaceholderText('规则链名称') as HTMLInputElement;
    // 回显已有名称
    expect(nameInput.value).toBe('订单处理链');
    // 修改名称 → 输入框值跟着变（不被旧 props 顶回），且 onChange 收到新值
    fireEvent.change(nameInput, { target: { value: '订单处理链-v2' } });
    expect(nameInput.value).toBe('订单处理链-v2');
    expect(patches[patches.length - 1].name).toBe('订单处理链-v2');
  });

  it('规则链描述可输入修改（本地受控）', () => {
    const patches: Array<{ description?: string }> = [];
    render(
      <SettingsFormLike name="x" description="旧描述" onChange={(p) => patches.push(p)} />,
    );
    const desc = screen.getByDisplayValue('旧描述') as HTMLTextAreaElement;
    fireEvent.change(desc, { target: { value: '新描述' } });
    expect(desc.value).toBe('新描述');
    expect(patches[patches.length - 1].description).toBe('新描述');
  });
});
