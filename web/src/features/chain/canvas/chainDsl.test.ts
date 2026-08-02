import { describe, it, expect } from 'vitest';
import { dslToFlow, flowToDsl, layoutFlow, isContainerType, type DslChain } from './chainDsl';

const sampleDsl: DslChain = {
  ruleChain: { id: 'chain_1', name: 'demo', root: true },
  metadata: {
    nodes: [
      { id: 'n1', type: 'jsTransform', name: '转换', configuration: { jsScript: 'return msg;' }, additionalInfo: { position: { x: 10, y: 20 } } },
      { id: 'n2', type: 'jsFilter', name: '过滤', configuration: {} },
      { id: 'loop', type: 'for', name: '遍历', configuration: {}, subChain: { metadata: { nodes: [{ id: 'sub1', type: 'log', name: '日志', configuration: {} }], connections: [] } } },
    ],
    connections: [
      { fromId: 'n1', toId: 'n2', type: 'Success' },
      { fromId: 'n2', toId: 'loop', type: 'True' },
    ],
  },
};

describe('isContainerType', () => {
  it('识别 for / flow 为容器', () => {
    expect(isContainerType('for')).toBe(true);
    expect(isContainerType('flow')).toBe(true);
    expect(isContainerType('jsTransform')).toBe(false);
  });
});

describe('dslToFlow', () => {
  it('转换节点与连线', () => {
    const { nodes, edges } = dslToFlow(sampleDsl);
    expect(nodes).toHaveLength(3);
    expect(edges).toHaveLength(2);
    expect(nodes[0].position).toEqual({ x: 10, y: 20 });
    expect(nodes[0].data.ruleType).toBe('jsTransform');
    // 容器节点
    const loop = nodes.find((n) => n.id === 'loop')!;
    expect(loop.type).toBe('container');
    expect((loop.data as { subFlow?: { nodes: unknown[] } }).subFlow?.nodes).toHaveLength(1);
    // 连线
    expect(edges[0].source).toBe('n1');
    expect(edges[0].target).toBe('n2');
    expect(edges[0].data?.relationType).toBe('Success');
  });

  it('无位置时给默认坐标', () => {
    const { nodes } = dslToFlow(sampleDsl);
    expect(nodes[1].position.x).toBeGreaterThan(0);
  });
});

describe('flowToDsl', () => {
  it('往返转换保持结构', () => {
    const { nodes, edges } = dslToFlow(sampleDsl);
    const dsl = flowToDsl({ id: 'chain_1', name: 'demo' }, nodes, edges);
    expect(dsl.metadata?.nodes).toHaveLength(3);
    expect(dsl.metadata?.connections).toHaveLength(2);
    const loop = dsl.metadata?.nodes?.find((n) => n.id === 'loop')!;
    expect(loop.subChain?.metadata?.nodes).toHaveLength(1);
    // 坐标写回 additionalInfo
    const n1 = dsl.metadata?.nodes?.find((n) => n.id === 'n1')!;
    expect(n1.additionalInfo?.position).toEqual({ x: 10, y: 20 });
  });
});

describe('layoutFlow', () => {
  it('为所有节点分配坐标', () => {
    const { nodes, edges } = dslToFlow(sampleDsl);
    // 清掉位置模拟无布局
    const noPos = nodes.map((n) => ({ ...n, position: { x: 0, y: 0 } }));
    const laid = layoutFlow(noPos, edges);
    expect(laid).toHaveLength(3);
    // dagre 应让节点分散开（不都在原点）
    const xs = new Set(laid.map((n) => Math.round(n.position.x)));
    expect(xs.size).toBeGreaterThan(1);
  });
});
