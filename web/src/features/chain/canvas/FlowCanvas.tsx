import { useCallback } from 'react';
import {
  ReactFlow, Background, Controls, MiniMap, addEdge, reconnectEdge,
  applyNodeChanges, applyEdgeChanges, useReactFlow,
  type Connection, type Edge, type Node, type NodeChange, type EdgeChange,
} from '@xyflow/react';
import { App } from 'antd';

import RuleNode from './nodes/RuleNode';
import ContainerNode from './nodes/ContainerNode';
import { ComponentMeta } from '@/api/component';
import { DND_MIME } from './ComponentPalette';
import {
  genNodeId,
  isContainerType,
  relationClassName,
  relationTypesForNode,
  RuleNodeData,
} from './chainDsl';
import { componentZhName } from './componentZh';
import { useCanvasStore } from '@/stores/canvasStore';

const nodeTypes = { rule: RuleNode, container: ContainerNode };

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
      const relationType = conn.sourceHandle ?? 'Success';
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
      const next = reconnectEdge(oldEdge, { ...conn, sourceHandle: relationType }, edges)
        .map((e) => e.id === oldEdge.id
          ? {
              ...e,
              label: relationType,
              data: { ...e.data, relationType },
              className: relationClassName(relationType),
            }
          : e);
      props.onEdgesChange(next);
    },
    [edges, message, nodes, props]
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
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={handleNodesChange}
        onEdgesChange={handleEdgesChange}
        onConnect={onConnect}
        onReconnect={onReconnect}
        onNodeClick={(_, node) => { setSelectedNodeId(node.id); props.onSelectNode(node); }}
        onPaneClick={() => { setSelectedNodeId(null); props.onSelectNode(null); }}
        onNodeDoubleClick={(_, node) => { if (node.type === 'container') props.onEnterSub(node.id); }}
        deleteKeyCode={['Delete', 'Backspace']}
        multiSelectionKeyCode={['Meta', 'Control']}
        selectionOnDrag
        panOnDrag={[1, 2]}
        reconnectRadius={12}
        fitView
        minZoom={0.2}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={16} size={1} color="#dfe3ee" />
        <MiniMap pannable zoomable style={{ width: 140, height: 90 }} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
