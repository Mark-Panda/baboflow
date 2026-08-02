import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { ReactFlowProvider, type NodeProps } from '@xyflow/react';
import RuleNode from './nodes/RuleNode';
import ContainerNode from './nodes/ContainerNode';

// jsdom 缺 ResizeObserver / DOMMatrixReadOnly，xyflow 渲染需要。
class RO { observe() {} unobserve() {} disconnect() {} }
(globalThis as { ResizeObserver?: unknown }).ResizeObserver = RO;

function propsFor(data: Record<string, unknown>): NodeProps {
  return {
    id: 'n1',
    data,
    selected: false,
    type: 'rule',
    isConnectable: true,
    positionAbsoluteX: 0,
    positionAbsoluteY: 0,
    dragging: false,
  } as unknown as NodeProps;
}

describe('RuleNode', () => {
  it('渲染名称与类型', () => {
    const { getByText } = render(
      <ReactFlowProvider>
        <RuleNode {...propsFor({ ruleType: 'jsTransform', name: 'JS转换', category: 'transform', configuration: {} })} />
      </ReactFlowProvider>
    );
    expect(getByText('JS转换')).toBeTruthy();
    expect(getByText('jsTransform')).toBeTruthy();
  });

  it('调试模式显示标记', () => {
    const { container } = render(
      <ReactFlowProvider>
        <RuleNode {...propsFor({ ruleType: 'log', name: '日志', category: 'action', configuration: {}, debugMode: true })} />
      </ReactFlowProvider>
    );
    expect(container.querySelector('.rn-debug')).toBeTruthy();
  });
});

describe('ContainerNode', () => {
  it('渲染容器并展示子节点数与进入入口', () => {
    const { getByText } = render(
      <ReactFlowProvider>
        <ContainerNode
          {...propsFor({
            ruleType: 'for', name: '遍历', category: 'common', configuration: {},
            subFlow: { nodes: [{ id: 'a' }, { id: 'b' }], edges: [] },
          })}
        />
      </ReactFlowProvider>
    );
    expect(getByText('遍历')).toBeTruthy();
    expect(getByText(/2 子节点/)).toBeTruthy();
    expect(getByText(/进入子画布/)).toBeTruthy();
  });
});
