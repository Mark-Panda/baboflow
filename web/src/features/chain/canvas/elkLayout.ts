// elkjs（ELK Layered）自动布局：节点分层 + 合理间距，替代原 dagre。
// 只重排节点坐标；边路由/绕障由自定义 AvoidEdge 负责（见 edges/routePath.ts）。
import ELK from 'elkjs/lib/elk.bundled.js';
import type { Edge, Node } from '@xyflow/react';

import { DEFAULT_RELATION_TYPES, type RuleNodeData } from './chainDsl';

const elk = new ELK();

// 节点尺寸兜底（未渲染测量时）；与 canvas.css 的 min-width 对齐。
const NODE_W = 200;
const NODE_H = 64;
const CONTAINER_W = 260;
const CONTAINER_H = 96;

function nodeSize(n: Node): { width: number; height: number } {
  const isC = n.type === 'container';
  const d = n.data as RuleNodeData;
  // 实测宽高优先（xyflow v12 渲染后回填 measured），其次按输出关系数撑高估计。
  const measuredW = n.measured?.width;
  const measuredH = n.measured?.height;
  const relationCount = d.relationTypes?.length ?? DEFAULT_RELATION_TYPES.length;
  const estH = Math.max(isC ? CONTAINER_H : NODE_H, relationCount * 24 + 32);
  return {
    width: measuredW ?? (isC ? CONTAINER_W : NODE_W),
    height: measuredH ?? estH,
  };
}

// 用 ELK layered 从左到右分层布局，返回带新 position 的节点数组（左上角坐标）。
export async function layoutFlowElk(nodes: Node[], edges: Edge[]): Promise<Node[]> {
  if (nodes.length === 0) return nodes;

  const graph = {
    id: 'root',
    layoutOptions: {
      'elk.algorithm': 'layered',
      'elk.direction': 'RIGHT',
      // 层间/同层间距，给绕障边留出通道
      'elk.layered.spacing.nodeNodeBetweenLayers': '90',
      'elk.spacing.nodeNode': '48',
      'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX',
      'elk.edgeRouting': 'ORTHOGONAL',
      // 尽量保持既有连边顺序，减少交叉
      'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES',
      'elk.layered.crossingMinimization.strategy': 'LAYER_SWEEP',
      'elk.padding': '[top=40,left=40,bottom=40,right=40]',
    },
    children: nodes.map((n) => ({ id: n.id, ...nodeSize(n) })),
    edges: edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  };

  const res = await elk.layout(graph);
  const posById = new Map<string, { x: number; y: number }>();
  (res.children ?? []).forEach((c) => {
    posById.set(c.id, { x: c.x ?? 0, y: c.y ?? 0 });
  });

  return nodes.map((n) => {
    const p = posById.get(n.id);
    if (!p) return n;
    return { ...n, position: { x: p.x, y: p.y } };
  });
}
