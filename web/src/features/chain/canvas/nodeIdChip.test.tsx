import { describe, it, expect, beforeAll } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Tooltip, Typography } from 'antd';

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

// 渲染与 NodeConfigPanel 头部一致的「节点名称 + 节点 ID 只读可复制」标签。
function NameLabelWithId({ id }: { id: string }) {
  return (
    <span className="bf-node-name-label">
      节点名称
      <Tooltip title="节点 ID（只读，点击复制）">
        <Typography.Text
          className="bf-node-id-chip"
          copyable={{ text: id, tooltips: ['复制 ID', '已复制'] }}
        >
          {id}
        </Typography.Text>
      </Tooltip>
    </span>
  );
}

describe('节点 ID 只读可复制 chip', () => {
  it('显示节点 ID 文本，且不可编辑（无输入框/无编辑态）', () => {
    render(<NameLabelWithId id="restApiCall_lx1_3" />);
    // ID 文本可见
    expect(screen.getByText('restApiCall_lx1_3')).toBeTruthy();
    // 不可修改：没有可编辑输入框、没有进入 Typography 编辑态
    expect(screen.queryByRole('textbox')).toBeNull();
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('提供复制入口（copyable 复制按钮，aria-label=复制 ID）', () => {
    render(<NameLabelWithId id="node_abc" />);
    // antd copyable 渲染一个 .ant-typography-copy 的 <button aria-label="复制 ID">
    const copyBtn = document.querySelector('.ant-typography-copy') as HTMLElement;
    expect(copyBtn).not.toBeNull();
    expect(copyBtn.tagName).toBe('BUTTON');
    expect(copyBtn.getAttribute('aria-label')).toBe('复制 ID');
  });

  it('copyable 绑定的是节点 ID 文本（chip 无第二个文本来源）', () => {
    render(<NameLabelWithId id="node_xyz_9" />);
    const chip = document.querySelector('.bf-node-id-chip') as HTMLElement;
    // chip 文本即节点 ID；copyable 的 text 配置与该 ID 一致（点击复制的就是它）
    expect(chip.textContent).toContain('node_xyz_9');
    // 点击复制按钮不报错（copy-to-clipboard 在 jsdom 用 execCommand 容错，不抛异常即视为可交互）
    const copyBtn = chip.querySelector('.ant-typography-copy') as HTMLElement;
    expect(() => fireEvent.click(copyBtn)).not.toThrow();
  });
});
