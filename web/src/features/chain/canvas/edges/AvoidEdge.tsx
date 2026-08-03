import { useMemo } from 'react';
import {
  BaseEdge,
  EdgeLabelRenderer,
  Position,
  getSmoothStepPath,
  useStore,
  type EdgeProps,
} from '@xyflow/react';
import { Dropdown } from 'antd';

import { relationZhName } from '../componentZh';
import { labelAnchorNearSource, routeAround, type Pt, type Rect } from './routePath';

// 边上注入的「关系下拉」数据（由 FlowCanvas 计算并经 edge.data 传入）。
interface EdgeRelationData {
  relationType?: string;
  __relationOptions?: string[];
  __onChangeRelation?: (edgeId: string, next: string) => void;
}

export interface EdgeRelationLabelProps {
  edgeId: string;
  label: React.ReactNode;
  selected?: boolean;
  labelStyle?: EdgeProps['labelStyle'];
  labelBgStyle?: EdgeProps['labelBgStyle'];
  relationType?: string;
  relationOptions?: string[];
  onChangeRelation?: (edgeId: string, next: string) => void;
}

// 连线中点的关系标签：可点时在源节点可用连接类型间切换（antd Dropdown），否则只读。
// 抽成独立组件以便脱离 xyflow 边渲染机制直接单测（EdgeLabelRenderer 的 portal 在 jsdom 不挂载）。
export function EdgeRelationLabel({
  edgeId,
  label,
  selected,
  labelStyle,
  labelBgStyle,
  relationType,
  relationOptions = [],
  onChangeRelation,
}: EdgeRelationLabelProps) {
  const currentRelation = relationType ?? (typeof label === 'string' ? label : undefined);
  // 可切换：有回调且可选关系多于一个；否则退化为只读标签。
  const switchable = !!onChangeRelation && relationOptions.length > 1;
  // 展示用中文；menu key / 回调参数 / DSL 仍用英文原值。
  const displayLabel = typeof label === 'string' ? relationZhName(label) : label;
  return (
    <Dropdown
      trigger={['click']}
      disabled={!switchable}
      menu={{
        items: relationOptions.map((relation) => ({
          key: relation,
          label: relationZhName(relation),
        })),
        selectedKeys: currentRelation ? [currentRelation] : [],
        onClick: ({ key }) => {
          if (key !== currentRelation) onChangeRelation?.(edgeId, key);
        },
      }}
    >
      <span
        className={`react-flow__edge-text ${switchable ? 'bf-edge-label' : ''}`}
        style={{
          background: (labelBgStyle as { fill?: string } | undefined)?.fill ?? '#fff',
          color: (labelStyle as { fill?: string } | undefined)?.fill,
          fontWeight: selected ? 600 : 400,
        }}
      >
        {displayLabel}
        {switchable ? ' ▾' : ''}
      </span>
    </Dropdown>
  );
}

// 自定义「绕障」边：先算出绕过中间节点的正交折线顶点，再用 smoothstep 取圆角渲染。
// 源/目标节点自身不作为障碍（xyflow 已把锚点给在其边缘），其余节点包围盒参与避让。
// 中点标签是可点的关系下拉：点击在源节点可用连接类型间切换。
export default function AvoidEdge(props: EdgeProps) {
  const {
    id,
    source,
    target,
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    label,
    labelStyle,
    labelBgStyle,
    selected,
  } = props;

  const relationData = props.data as EdgeRelationData | undefined;

  // 订阅节点查找表：节点增删/移动时重算绕障路径。
  const obstacles = useStore((store) => {
    const rects: Rect[] = [];
    store.nodeLookup.forEach((internal) => {
      if (internal.id === source || internal.id === target) return;
      const w = internal.measured?.width ?? 200;
      const h = internal.measured?.height ?? 64;
      const pos = internal.internals?.positionAbsolute ?? internal.position;
      rects.push({ x: pos.x, y: pos.y, width: w, height: h });
    });
    return rects;
  });

  const pts = useMemo<Pt[]>(
    () => routeAround({ x: sourceX, y: sourceY }, { x: targetX, y: targetY }, obstacles),
    [sourceX, sourceY, targetX, targetY, obstacles],
  );

  // 用中点拐点约束 smoothstep，让它贴近绕障折线；圆角让过渡自然。
  const mid = pts.length >= 2 ? pts[Math.floor(pts.length / 2)] : undefined;
  const [edgePath] = getSmoothStepPath({
    sourceX,
    sourceY,
    sourcePosition: sourcePosition ?? Position.Right,
    targetX,
    targetY,
    targetPosition: targetPosition ?? Position.Left,
    borderRadius: 16,
    centerX: mid?.x,
    centerY: mid?.y,
    offset: 20,
  });

  // 标签锚在源端首段（每条连线独有），避免多线汇入同目标时在几何中心重叠。
  const { x: labelX, y: labelY } = labelAnchorNearSource(pts);

  return (
    <>
      <BaseEdge id={id} path={edgePath} />
      {label != null && (
        <EdgeLabelRenderer>
          <div
            className="react-flow__edge-textwrapper"
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              pointerEvents: 'all',
            }}
          >
            <EdgeRelationLabel
              edgeId={id}
              label={label}
              selected={selected}
              labelStyle={labelStyle}
              labelBgStyle={labelBgStyle}
              relationType={relationData?.relationType}
              relationOptions={relationData?.__relationOptions}
              onChangeRelation={relationData?.__onChangeRelation}
            />
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}
