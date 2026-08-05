import { describe, expect, it } from 'vitest';
import type { Edge, Node } from '@xyflow/react';

import { layoutFlowElk, layoutFlowTree } from './elkLayout';

describe('layoutFlowTree', () => {
  it('递归布局容器节点的子画布', async () => {
    const nodes = [{
      id: 'loop',
      type: 'container',
      position: { x: 0, y: 0 },
      data: {
        ruleType: 'for',
        name: '遍历',
        relationTypes: ['Success'],
        subFlow: {
          nodes: [{
            id: 'sub',
            type: 'rule',
            position: { x: 0, y: 0 },
            data: { ruleType: 'log', name: '日志', relationTypes: ['Success'] },
          }],
          edges: [],
        },
      },
    }] as never[];

    const laid = await layoutFlowTree(nodes, []);
    const subFlow = (laid[0].data as { subFlow: { nodes: Node[] } }).subFlow;
    expect(subFlow.nodes[0].position).toEqual(expect.objectContaining({
      x: expect.any(Number),
      y: expect.any(Number),
    }));
  });
});

function n(id: string, x = 0, y = 0): Node {
  return { id, type: 'rule', position: { x, y }, data: { ruleType: 'log', name: id, category: 'action', configuration: {} } } as Node;
}
function e(id: string, source: string, target: string): Edge {
  return { id, source, target } as Edge;
}

describe('layoutFlowElk', () => {
  it('空数组原样返回', async () => {
    expect(await layoutFlowElk([], [])).toEqual([]);
  });

  it('链式 A→B→C：从左到右 x 递增，无重叠', async () => {
    const nodes = [n('A'), n('B'), n('C')];
    const edges = [e('e1', 'A', 'B'), e('e2', 'B', 'C')];
    const out = await layoutFlowElk(nodes, edges);
    const pos = Object.fromEntries(out.map((x) => [x.id, x.position]));
    // 从左到右分层：x 严格递增
    expect(pos.A.x).toBeLessThan(pos.B.x);
    expect(pos.B.x).toBeLessThan(pos.C.x);
  });

  it('分支：同一源的两个目标分在不同行（y 不同），x 同层', async () => {
    const nodes = [n('S'), n('T1'), n('T2')];
    const edges = [e('e1', 'S', 'T1'), e('e2', 'S', 'T2')];
    const out = await layoutFlowElk(nodes, edges);
    const pos = Object.fromEntries(out.map((x) => [x.id, x.position]));
    // T1、T2 同层（x 相同），但纵向错开（y 不同）
    expect(pos.T1.x).toBeCloseTo(pos.T2.x, 0);
    expect(pos.T1.y).not.toBeCloseTo(pos.T2.y, 0);
  });

  it('使用 measured 宽高（渲染后实测）参与布局', async () => {
    const big = {
      ...n('big'),
      measured: { width: 400, height: 200 },
    } as Node;
    const out = await layoutFlowElk([big, n('next')], [e('e1', 'big', 'next')]);
    const pos = Object.fromEntries(out.map((x) => [x.id, x.position]));
    // next 应在 big 右侧（留出不小于大节点宽度的水平间距）
    expect(pos.next.x).toBeGreaterThan(pos.big.x);
  });
});
