// 规则链画布：RuleGo DSL <-> ReactFlow 互转。
import type { Edge, Node } from '@xyflow/react';
import type { ComponentMeta } from '@/api/component';

// 自定义绕障边类型（AvoidEdge），dslToFlow/新建连线统一用它渲染。
export const EDGE_TYPE = 'avoid';

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
  relationTypes?: string[];
  // 容器节点的子画布（内存态，保存时序列化进 DSL）
  subFlow?: { nodes: Node[]; edges: Edge[] };
}

export const DEFAULT_RELATION_TYPES = ['Success', 'Failure'];

function uniqueRelations(relations: unknown[]): string[] {
  return [...new Set(
    relations.filter((value): value is string =>
      typeof value === 'string' && value.trim().length > 0
    ).map((value) => value.trim()),
  )];
}

export function relationTypesForNode(
  ruleType: string,
  configuration: Record<string, unknown> = {},
  components: ComponentMeta[] = [],
  existingRelations: string[] = [],
): string[] {
  if (ruleType === 'switch') {
    const cases = Array.isArray(configuration.cases) ? configuration.cases : [];
    const caseRelations = cases.map((item) =>
      item && typeof item === 'object' && 'then' in item
        ? (item as { then?: unknown }).then
        : undefined
    );
    return uniqueRelations([...caseRelations, 'Default', 'Failure', ...existingRelations]);
  }
  const relationTypes = components.find((c) => c.type === ruleType)?.configSchema?.relationTypes;
  if (Array.isArray(relationTypes) && relationTypes.length > 0) {
    return uniqueRelations([...relationTypes, ...existingRelations]);
  }
  return uniqueRelations([...DEFAULT_RELATION_TYPES, ...existingRelations]);
}

export function relationClassName(relationType: string): string {
  const safe = relationType.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  return `edge-${safe || 'success'}`;
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
export function dslToFlow(
  dsl: DslChain,
  components: ComponentMeta[] = [],
): { nodes: Node[]; edges: Edge[] } {
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
        relationTypes: relationTypesForNode(n.type, n.configuration ?? {}, components),
        subFlow: isContainer && n.subChain ? dslToFlow(n.subChain, components) : undefined,
      } satisfies RuleNodeData,
    };
  });
  const edges: Edge[] = (meta.connections ?? []).map((c) => {
    const relationType = c.type || 'Success';
    return {
      id: `${c.fromId}->${c.toId}:${relationType}`,
      type: EDGE_TYPE,
      source: c.fromId,
      target: c.toId,
      sourceHandle: relationType,
      label: relationType,
      data: { relationType },
      className: relationClassName(relationType),
    };
  });
  const relationTypesByNode = new Map<string, Set<string>>();
  edges.forEach((edge) => {
    const relationType = edge.data?.relationType;
    if (typeof relationType !== 'string') return;
    const types = relationTypesByNode.get(edge.source) ?? new Set<string>();
    types.add(relationType);
    relationTypesByNode.set(edge.source, types);
  });
  const nodesWithRelations = nodes.map((node) => {
    const d = node.data as RuleNodeData;
    const fromDsl = relationTypesByNode.get(node.id);
    if (!fromDsl) return node;
    return {
      ...node,
      data: {
        ...d,
        relationTypes: relationTypesForNode(d.ruleType, d.configuration, components, [
          ...(d.relationTypes ?? []),
          ...fromDsl,
        ]),
      } satisfies RuleNodeData,
    };
  });
  return { nodes: nodesWithRelations, edges };
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

// ---- 自动布局已迁移至 elkLayout.ts（ELK layered）----
// 节点尺寸估计常量与布局实现见 elkLayout.ts；此处不再依赖 dagre。

let seq = 0;
export function genNodeId(ruleType: string): string {
  seq += 1;
  const safe = ruleType.replace(/[^a-zA-Z0-9]/g, '_');
  return `${safe}_${Date.now().toString(36)}_${seq}`;
}
