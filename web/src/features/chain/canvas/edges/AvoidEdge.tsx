import { useMemo } from 'react';
import {
  BaseEdge,
  EdgeLabelRenderer,
  Position,
  getSmoothStepPath,
  useStore,
  type EdgeProps,
} from '@xyflow/react';

import { routeAround, type Pt, type Rect } from './routePath';

// 自定义「绕障」边：先算出绕过中间节点的正交折线顶点，再用 smoothstep 取圆角渲染。
// 源/目标节点自身不作为障碍（xyflow 已把锚点给在其边缘），其余节点包围盒参与避让。
export default function AvoidEdge(props: EdgeProps) {
  const {
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
  const [edgePath, labelX, labelY] = getSmoothStepPath({
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

  return (
    <>
      <BaseEdge id={props.id} path={edgePath} />
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
            <span
              className="react-flow__edge-text"
              style={{
                background: (labelBgStyle as { fill?: string } | undefined)?.fill ?? '#fff',
                color: (labelStyle as { fill?: string } | undefined)?.fill,
                fontWeight: selected ? 600 : 400,
              }}
            >
              {label}
            </span>
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}
