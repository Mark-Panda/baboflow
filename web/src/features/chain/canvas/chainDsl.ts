// 规则链画布：RuleGo DSL <-> ReactFlow 互转、dagre 自动布局。
import dagre from 'dagre';
import type { Edge, Node } from '@xyflow/react';

// ---- RuleGo DSL 类型 ----
export interface DslNode {
  id: string;
  type: string;
  name?: string;
  debugMode?: boolean;
  configuration?: Record<string, unknown>;
  additionalInfo?: { position?: { x: number; y: number }; [k: string]: unknown };
  // for 容器节点的子链（RuleGo 嵌套）
  subChain?: DslChain;
}
export interface DslConnection {
  fromId: string;
  toId: string;
  type: string;
}
export interface DslChain {
  ruleChain?: Record<string, unknown>;
  metadata?: {
    nodes?: DslNode[];
    connections?: DslConnection[];
  };
}

// ---- ReactFlow 节点 data ----
export interface RuleNodeData extends Record<string, unknown> {
  ruleType: string;
  name: string;
  category: string;
  configuration: Record<string, unknown>;
  debugMode?: boolean;
  // 容器节点的子画布（内存态，保存时序列化进 DSL）
  subFlow?: { nodes: Node[]; edges: Edge[] };
}

export const CONTAINER_TYPES = new Set(['for', 'flow']);
export function isContainerType(ruleType: string): boolean {
  return CONTAINER_TYPES.has(ruleType);
}

export function categoryOf(ruleType: string): string {
  // 内置组件 type 形如 "jsTransform"、"restApiCall"、"for"；类别由后端 ComponentMeta 提供，
  // 这里给一个回退推断（找不到时归 common）。
  return ruleType.includes('/') ? ruleType.split('/')[0] : '';
}

// ---- DSL -> ReactFlow ----
export function dslToFlow(dsl: DslChain): { nodes: Node[]; edges: Edge[] } {
  const meta = dsl?.metadata ?? {};
  const nodes: Node[] = (meta.nodes ?? []).map((n, i) => {
    const isContainer = isContainerType(n.type);
    return {
      id: n.id,
      type: isContainer ? 'container' : 'rule',
      position: n.additionalInfo?.position ?? { x: 120 + (i % 4) * 230, y: 120 + Math.floor(i / 4) * 120 },
      data: {
        ruleType: n.type,
        name: n.name ?? n.type,
        category: categoryOf(n.type),
        configuration: n.configuration ?? {},
        debugMode: !!n.debugMode,
        subFlow: isContainer && n.subChain ? dslToFlow(n.subChain) : undefined,
      } satisfies RuleNodeData,
    };
  });
  const edges: Edge[] = (meta.connections ?? []).map((c) => ({
    id: `${c.fromId}->${c.toId}:${c.type}`,
    source: c.fromId,
    target: c.toId,
    label: c.type,
    data: { relationType: c.type },
    className: `edge-${String(c.type).toLowerCase()}`,
  }));
  return { nodes, edges };
}

// ---- ReactFlow -> DSL ----
export function flowToDsl(
  chainMeta: Record<string, unknown>,
  nodes: Node[],
  edges: Edge[]
): DslChain {
  return {
    ruleChain: { ...chainMeta, root: true },
    metadata: {
      nodes: nodes.map((n) => {
        const d = n.data as RuleNodeData;
        const base: DslNode = {
          id: n.id,
          type: d.ruleType,
          name: d.name,
          debugMode: !!d.debugMode,
          configuration: d.configuration ?? {},
          additionalInfo: { position: n.position },
        };
        if (isContainerType(d.ruleType) && d.subFlow) {
          // 递归序列化子画布
          base.subChain = {
            ruleChain: { id: n.id, name: d.name },
            metadata: {
              nodes: flowToDsl({}, d.subFlow.nodes, d.subFlow.edges).metadata?.nodes ?? [],
              connections:
                flowToDsl({}, d.subFlow.nodes, d.subFlow.edges).metadata?.connections ?? [],
            },
          };
        }
        return base;
      }),
      connections: edges.map((e) => ({
        fromId: e.source,
        toId: e.target,
        type: (e.data?.relationType as string) ?? 'Success',
      })),
    },
  };
}

// ---- dagre 自动布局（DAG 分层）----
const NODE_W = 200;
const NODE_H = 64;
const CONTAINER_W = 260;
const CONTAINER_H = 96;

export function layoutFlow(nodes: Node[], edges: Edge[]): Node[] {
  if (nodes.length === 0) return nodes;
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: 'LR', nodesep: 60, ranksep: 120, marginx: 40, marginy: 40 });
  g.setDefaultEdgeLabel(() => ({}));
  nodes.forEach((n) => {
    const isC = n.type === 'container';
    g.setNode(n.id, { width: isC ? CONTAINER_W : NODE_W, height: isC ? CONTAINER_H : NODE_H });
  });
  edges.forEach((e) => g.setEdge(e.source, e.target));
  dagre.layout(g);
  return nodes.map((n) => {
    const pos = g.node(n.id);
    if (!pos) return n;
    const isC = n.type === 'container';
    const w = isC ? CONTAINER_W : NODE_W;
    const h = isC ? CONTAINER_H : NODE_H;
    return { ...n, position: { x: pos.x - w / 2, y: pos.y - h / 2 } };
  });
}

let seq = 0;
export function genNodeId(ruleType: string): string {
  seq += 1;
  const safe = ruleType.replace(/[^a-zA-Z0-9]/g, '_');
  return `${safe}_${Date.now().toString(36)}_${seq}`;
}
