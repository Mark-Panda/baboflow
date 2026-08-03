import { describe, it, expect, beforeAll, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import { EdgeRelationLabel } from './edges/AvoidEdge';

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

describe('EdgeRelationLabel 关系下拉', () => {
  it('标签显示中文，点击切换后回调仍用英文原值', async () => {
    const onChangeRelation = vi.fn();
    render(
      <EdgeRelationLabel
        edgeId="e1"
        label="Success"
        relationType="Success"
        relationOptions={['Success', 'Failure']}
        onChangeRelation={onChangeRelation}
      />
    );

    // 标签显示中文「成功」而非英文 Success
    fireEvent.click(screen.getByText(/成功/));

    // 下拉项显示中文「失败」，点击它
    const item = await screen.findByText('失败');
    fireEvent.click(item);

    // 回调传的仍是英文原值 Failure（DSL/引擎契约不变）
    await waitFor(() => expect(onChangeRelation).toHaveBeenCalledWith('e1', 'Failure'));
  });

  it('重复选择当前关系不触发回调', async () => {
    const onChangeRelation = vi.fn();
    render(
      <EdgeRelationLabel
        edgeId="e1"
        label="Success"
        relationType="Success"
        relationOptions={['Success', 'Failure']}
        onChangeRelation={onChangeRelation}
      />
    );

    fireEvent.click(screen.getByText(/成功/));
    // 菜单里的当前项（aria-selected）——重复选择不触发 onChangeRelation
    const menuItem = await screen.findByRole('menuitem', { name: '成功' });
    fireEvent.click(menuItem);

    await waitFor(() => expect(onChangeRelation).not.toHaveBeenCalled());
  });

  it('switch 自定义分支名不在映射表 → 原样显示', () => {
    render(
      <EdgeRelationLabel
        edgeId="e1"
        label="Case1"
        relationType="Case1"
        relationOptions={['Case1']}
        onChangeRelation={vi.fn()}
      />
    );
    expect(screen.getByText('Case1')).toBeTruthy();
  });

  it('单一可用关系时退化为只读标签（无箭头、无可点样式）', () => {
    render(
      <EdgeRelationLabel
        edgeId="e1"
        label="Success"
        relationType="Success"
        relationOptions={['Success']}
        onChangeRelation={vi.fn()}
      />
    );
    const label = screen.getByText('成功');
    expect(label.textContent).toBe('成功');
    expect(label.className).not.toContain('bf-edge-label');
  });

  it('无切换回调时退化为只读标签', () => {
    render(
      <EdgeRelationLabel
        edgeId="e1"
        label="Success"
        relationType="Success"
        relationOptions={['Success', 'Failure']}
      />
    );
    const label = screen.getByText('成功');
    expect(label.className).not.toContain('bf-edge-label');
  });
});
