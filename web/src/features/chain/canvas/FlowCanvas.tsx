import { useCallback, useMemo } from 'react';
import {
  ReactFlow, Background, Controls, MiniMap, addEdge, reconnectEdge,
  applyNodeChanges, applyEdgeChanges, useReactFlow,
  ConnectionLineType,
  type Connection, type Edge, type Node, type NodeChange, type EdgeChange,
} from '@xyflow/react';
import { App } from 'antd';

import RuleNode from './nodes/RuleNode';
import ContainerNode from './nodes/ContainerNode';
import AvoidEdge from './edges/AvoidEdge';
import { ComponentMeta } from '@/api/component';
import { DND_MIME } from './ComponentPalette';
import {
  EDGE_TYPE,
  availableRelationsForEdge,
  defaultRelationFor,
  genNodeId,
  isContainerType,
  relationClassName,
  relationTypesForNode,
  RuleNodeData,
} from './chainDsl';
import { componentZhName } from './componentZh';
import { useCanvasStore } from '@/stores/canvasStore';

const nodeTypes = { rule: RuleNode, container: ContainerNode };
const edgeTypes = { [EDGE_TYPE]: AvoidEdge };

function edgeRelationType(edge: Edge): string {
  const relationType = (edge.data as { relationType?: unknown } | undefined)?.relationType;
  return typeof relationType === 'string' ? relationType : 'Success';
}

export interface FlowCanvasProps {
  nodes: Node[];
  edges: Edge[];
  components: ComponentMeta[];
  onNodesChange: (nodes: Node[]) => void;
  onEdgesChange: (edges: Edge[]) => void;
  onSelectNode: (node: Node | null) => void;
  onEnterSub: (nodeId: string) => void;
}

// 受控画布：父级（ChainEditorPage）持有 nodes/edges 单一事实源，便于子画布栈管理。
export default function FlowCanvas(props: FlowCanvasProps) {
  const { message } = App.useApp();
  const { screenToFlowPosition } = useReactFlow();
  const setSelectedNodeId = useCanvasStore((s) => s.setSelectedNodeId);

  const { nodes, edges } = props;

  const handleNodesChange = useCallback(
    (changes: NodeChange[]) => props.onNodesChange(applyNodeChanges(changes, nodes)),
    [nodes, props]
  );
  const handleEdgesChange = useCallback(
    (changes: EdgeChange[]) => props.onEdgesChange(applyEdgeChanges(changes, edges)),
    [edges, props]
  );

  const onConnect = useCallback(
    (conn: Connection) => {
      if (!conn.source || !conn.target) return;
      if (conn.source === conn.target) {
        message.warning('不允许自环连线');
        return;
      }
      // 节点为单一输出端，conn.sourceHandle 为空；连接类型取默认（Success 或第一个可用），
      // 之后可在连线上点击切换（见 AvoidEdge）。
      const sourceNode = nodes.find((node) => node.id === conn.source);
      const sourceData = sourceNode?.data as RuleNodeData | undefined;
      const relationTypes = sourceData
        ? relationTypesForNode(
            sourceData.ruleType,
            sourceData.configuration,
            props.components,
            sourceData.relationTypes,
          )
        : [];
      const relationType = defaultRelationFor(relationTypes);
      if (relationTypes?.length && !relationTypes.includes(relationType)) {
        message.warning('该输出关系不属于当前组件');
        return;
      }
      if (edges.some((e) =>
        e.source === conn.source
        && e.target === conn.target
        && edgeRelationType(e) === relationType
      )) {
        message.warning('两节点间该关系已存在连线');
        return;
      }
      props.onEdgesChange(
        addEdge({
          ...conn,
          type: EDGE_TYPE,
          label: relationType,
          data: { relationType },
          className: relationClassName(relationType),
        }, edges)
      );
    },
    [edges, message, nodes, props]
  );

  const onReconnect = useCallback(
    (oldEdge: Edge, conn: Connection) => {
      if (conn.source === conn.target) {
        message.warning('不允许自环连线');
        return;
      }
      const relationType = conn.sourceHandle ?? edgeRelationType(oldEdge);
      const sourceNode = nodes.find((node) => node.id === conn.source);
      const sourceData = sourceNode?.data as RuleNodeData | undefined;
      const relationTypes = sourceData
        ? relationTypesForNode(
            sourceData.ruleType,
            sourceData.configuration,
            props.components,
            sourceData.relationTypes,
          )
        : [];
      if (relationTypes?.length && !relationTypes.includes(relationType)) {
        message.warning('该输出关系不属于当前组件');
        return;
      }
      if (edges.some((e) =>
        e.id !== oldEdge.id
        && e.source === conn.source
        && e.target === conn.target
        && edgeRelationType(e) === relationType
      )) {
        message.warning('两节点间该关系已存在连线');
        return;
      }
      const next = reconnectEdge(oldEdge, { ...conn }, edges)
        .map((e) => e.id === oldEdge.id
          ? {
              ...e,
              type: EDGE_TYPE,
              label: relationType,
              data: { ...e.data, relationType },
              className: relationClassName(relationType),
            }
          : e);
      props.onEdgesChange(next);
    },
    [edges, message, nodes, props]
  );

  // 切换某条连线的连接类型：只更新 label/data.relationType/className，保持边 id 稳定，
  // 避免破坏 xyflow reconciliation 与选中态。受控画布下必须经 props.onEdgesChange 写回。
  const onChangeEdgeRelation = useCallback(
    (edgeId: string, next: string) => {
      props.onEdgesChange(
        edges.map((e) => e.id === edgeId
          ? {
              ...e,
              label: next,
              data: { ...e.data, relationType: next },
              className: relationClassName(next),
            }
          : e)
      );
    },
    [edges, props]
  );

  // 注入每条边的「关系下拉」所需数据（可用选项 + 切换回调），供 AvoidEdge 渲染交互标签。
  // 选项按同一 (source,target) 其他连线已占用的关系去重，避免重复三元组。
  const rfEdges = useMemo(
    () => edges.map((edge) => {
      const sourceData = nodes.find((node) => node.id === edge.source)?.data as
        | RuleNodeData
        | undefined;
      if (!sourceData) return edge;
      const allRelations = relationTypesForNode(
        sourceData.ruleType,
        sourceData.configuration,
        props.components,
        sourceData.relationTypes,
      );
      const siblingUsed = edges
        .filter((o) => o.id !== edge.id && o.source === edge.source && o.target === edge.target)
        .map((o) => edgeRelationType(o));
      return {
        ...edge,
        data: {
          ...edge.data,
          __relationOptions: availableRelationsForEdge(allRelations, siblingUsed),
          __onChangeRelation: onChangeEdgeRelation,
        },
      };
    }),
    [edges, nodes, props.components, onChangeEdgeRelation]
  );

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      const raw = e.dataTransfer.getData(DND_MIME);
      if (!raw) return;
      const comp: ComponentMeta = JSON.parse(raw);
      const pos = screenToFlowPosition({ x: e.clientX, y: e.clientY });
      const isContainer = isContainerType(comp.type);
      const node: Node = {
        id: genNodeId(comp.type),
        type: isContainer ? 'container' : 'rule',
        position: pos,
        data: {
          ruleType: comp.type,
          name: componentZhName(comp.type),
          category: comp.category,
          configuration: { ...(comp.example ?? {}) },
          debugMode: false,
          relationTypes: relationTypesForNode(comp.type, comp.example ?? {}, props.components),
          subFlow: isContainer ? { nodes: [], edges: [] } : undefined,
        } satisfies RuleNodeData,
      };
      props.onNodesChange([...nodes, node]);
    },
    [screenToFlowPosition, props, nodes]
  );

  return (
    <div
      className="bf-canvas-wrap"
      onDrop={onDrop}
      onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; }}
    >
      <ReactFlow
        nodes={nodes.map((n) => ({ ...n, data: { ...n.data, __onEnterSub: props.onEnterSub } }))}
        edges={rfEdges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        defaultEdgeOptions={{ type: EDGE_TYPE }}
        connectionLineType={ConnectionLineType.SmoothStep}
        onNodesChange={handleNodesChange}
        onEdgesChange={handleEdgesChange}
        onConnect={onConnect}
        onReconnect={onReconnect}
        onNodeClick={(_, node) => { setSelectedNodeId(node.id); props.onSelectNode(node); }}
        onPaneClick={() => { setSelectedNodeId(null); props.onSelectNode(null); }}
        onNodeDoubleClick={(_, node) => { if (node.type === 'container') props.onEnterSub(node.id); }}
        deleteKeyCode={['Delete', 'Backspace']}
        multiSelectionKeyCode={['Meta', 'Control']}
        selectionKeyCode={['Meta', 'Control']}
        panOnDrag
        selectionOnDrag={false}
        reconnectRadius={12}
        fitView
        minZoom={0.2}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={16} size={1} color="#dfe3ee" />
        <MiniMap pannable zoomable style={{ width: 140, height: 90 }} />
        <Controls showInteractive={false} />
        <div className="bf-canvas-hint">拖动空白平移 · 按住 ⌘/Ctrl 框选 · 滚轮缩放</div>
      </ReactFlow>
    </div>
  );
}
