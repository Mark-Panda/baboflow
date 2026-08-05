import type { Edge, Node } from '@xyflow/react';

import type { DslChain } from './chainDsl';

export interface CanvasFrame {
  nodes: Node[];
  edges: Edge[];
}

export interface CanvasDiff {
  addedNodes: number;
  removedNodes: number;
  changedNodes: number;
  addedEdges: number;
  removedEdges: number;
}

function nodeKey(node: Node): string {
  return JSON.stringify({
    id: node.id,
    type: node.type,
    position: node.position,
    data: node.data,
  });
}

function edgeKey(edge: Edge): string {
  return JSON.stringify({
    source: edge.source,
    target: edge.target,
    relationType: edge.data?.relationType ?? 'Success',
  });
}

export function flattenCanvas(root: CanvasFrame): CanvasFrame {
  const nodes: Node[] = [];
  const edges: Edge[] = [];
  const walk = (frame: CanvasFrame) => {
    nodes.push(...frame.nodes);
    edges.push(...frame.edges);
    frame.nodes.forEach((node) => {
      const subFlow = (node.data as { subFlow?: CanvasFrame } | undefined)?.subFlow;
      if (subFlow) walk(subFlow);
    });
  };
  walk(root);
  return { nodes, edges };
}

export function summarizeCanvasDiff(before: CanvasFrame, after: CanvasFrame): CanvasDiff {
  const oldCanvas = flattenCanvas(before);
  const newCanvas = flattenCanvas(after);
  const oldNodes = new Map(oldCanvas.nodes.map((node) => [node.id, node]));
  const newNodes = new Map(newCanvas.nodes.map((node) => [node.id, node]));
  const oldEdges = new Set(oldCanvas.edges.map(edgeKey));
  const newEdges = new Set(newCanvas.edges.map(edgeKey));

  let changedNodes = 0;
  newNodes.forEach((node, id) => {
    const previous = oldNodes.get(id);
    if (previous && nodeKey(previous) !== nodeKey(node)) changedNodes += 1;
  });

  return {
    addedNodes: [...newNodes.keys()].filter((id) => !oldNodes.has(id)).length,
    removedNodes: [...oldNodes.keys()].filter((id) => !newNodes.has(id)).length,
    changedNodes,
    addedEdges: [...newEdges].filter((edge) => !oldEdges.has(edge)).length,
    removedEdges: [...oldEdges].filter((edge) => !newEdges.has(edge)).length,
  };
}

function hasExplicitPositionsInDsl(dsl: DslChain): boolean {
  const nodes = dsl.metadata?.nodes ?? [];
  return nodes.every((node) => {
    const position = node.additionalInfo?.position;
    if (!position || !Number.isFinite(position.x) || !Number.isFinite(position.y)) return false;
    return !node.subChain || hasExplicitPositionsInDsl(node.subChain);
  });
}

export function shouldAutoLayout(dsl: DslChain, before: CanvasFrame, after: CanvasFrame): boolean {
  const diff = summarizeCanvasDiff(before, after);
  const structureChanged =
    diff.addedNodes > 0 ||
    diff.removedNodes > 0 ||
    diff.addedEdges > 0 ||
    diff.removedEdges > 0;
  return structureChanged && !hasExplicitPositionsInDsl(dsl);
}
