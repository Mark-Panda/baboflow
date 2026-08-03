import { describe, it, expect } from 'vitest';
import {
  dslToFlow,
  flowToDsl,
  EDGE_TYPE,
  availableRelationsForEdge,
  defaultRelationFor,
  isContainerType,
  relationTypesForNode,
  type DslChain,
} from './chainDsl';

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
  it('根据 switch cases 动态生成关系端口', () => {
    expect(relationTypesForNode('switch', {
      cases: [
        { case: 'msg.kind == "a"', then: 'Case1' },
        { case: 'msg.kind == "b"', then: 'Case2' },
        { case: 'msg.kind == "a"', then: 'Case1' },
      ],
    })).toEqual(['Case1', 'Case2', 'Default', 'Failure']);
  });

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
    // 连线（自定义绕障边类型）
    expect(edges[0].source).toBe('n1');
    expect(edges[0].target).toBe('n2');
    expect(edges[0].type).toBe(EDGE_TYPE);
    expect(edges[0].data?.relationType).toBe('Success');
    // 节点为单一无 id 输出端，不再写 sourceHandle；关系类型只在 data.relationType
    expect(edges[0].sourceHandle).toBeUndefined();
  });

  it('从组件 schema 恢复多个输出关系', () => {
    const { nodes } = dslToFlow(sampleDsl, [{
      type: 'jsFilter',
      configSchema: { relationTypes: ['True', 'False', 'Failure'] },
    }] as never);
    const filter = nodes.find((node) => node.id === 'n2')!;
    expect(filter.data.relationTypes).toEqual(['True', 'False', 'Failure']);
  });

  it('旧 DSL 的自定义关系会补回 switch 输出端口', () => {
    const dsl: DslChain = {
      metadata: {
        nodes: [{ id: 's1', type: 'switch', configuration: { cases: [] } }],
        connections: [{ fromId: 's1', toId: 'n1', type: 'LegacyCase' }],
      },
    };
    const { nodes } = dslToFlow(dsl);
    expect(nodes[0].data.relationTypes).toEqual(['Default', 'Failure', 'LegacyCase']);
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

  it('允许同一目标节点存在多个关系分支', () => {
    const { nodes } = dslToFlow(sampleDsl);
    const dsl = flowToDsl({}, nodes, [
      {
        id: 'n1->loop:True',
        source: 'n1',
        target: 'loop',
        data: { relationType: 'True' },
      },
      {
        id: 'n1->loop:False',
        source: 'n1',
        target: 'loop',
        data: { relationType: 'False' },
      },
    ]);
    expect(dsl.metadata?.connections?.map((connection) => connection.type))
      .toEqual(['True', 'False']);
  });
});

describe('defaultRelationFor', () => {
  it('优先返回 Success', () => {
    expect(defaultRelationFor(['Success', 'Failure'])).toBe('Success');
    expect(defaultRelationFor(['Failure', 'Success'])).toBe('Success');
  });

  it('无 Success 时取第一个可用关系（如 switch）', () => {
    expect(defaultRelationFor(['Case1', 'Default', 'Failure'])).toBe('Case1');
    expect(defaultRelationFor(['True', 'False', 'Failure'])).toBe('True');
  });

  it('空列表兜底 Success', () => {
    expect(defaultRelationFor([])).toBe('Success');
  });
});

describe('availableRelationsForEdge', () => {
  it('排除同 source+target 其他连线已占用的关系', () => {
    expect(availableRelationsForEdge(['Success', 'Failure'], ['Success']))
      .toEqual(['Failure']);
  });

  it('本边当前关系不在 siblingUsed 中，保持可选', () => {
    // 第二条边当前为 Failure；第一条占用 Success → 本边可选不含 Success 但含 Failure
    expect(availableRelationsForEdge(['Success', 'Failure'], ['Success']))
      .toContain('Failure');
  });

  it('无占用时返回全部', () => {
    expect(availableRelationsForEdge(['Success', 'Failure'], []))
      .toEqual(['Success', 'Failure']);
  });
});
