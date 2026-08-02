import { memo } from 'react';
import { Handle, Position, NodeProps } from '@xyflow/react';
import { Tooltip } from 'antd';
import { BugOutlined, LoginOutlined } from '@ant-design/icons';

import { RuleNodeData } from '../chainDsl';
import { catStyle } from '../category';
import { useCanvasStore } from '@/stores/canvasStore';
import { inferCat } from './RuleNode';

// for / flow 容器节点：更大卡片，带「进入子画布」入口。
function ContainerNodeInner({ id, data, selected }: NodeProps) {
  const d = data as RuleNodeData;
  const cat = catStyle(d.category || inferCat(d.ruleType));
  const state = useCanvasStore((s) => s.nodeStates[id]);
  const onEnterSub = (d as { __onEnterSub?: (id: string) => void }).__onEnterSub;

  const subCount = d.subFlow?.nodes?.length ?? 0;
  const kindLabel = d.ruleType === 'for' ? '遍历循环' : '子链';

  return (
    <div
      className={`rule-node container ${selected ? 'selected' : ''} ${state ? `st-${state}` : ''}`}
      style={{ ['--cat-color' as string]: cat.color }}
      onDoubleClick={() => onEnterSub?.(id)}
    >
      <Handle type="target" position={Position.Left} />
      <div className="rn-head">
        <span className="rn-icon">{cat.icon}</span>
        <span className="rn-name" title={d.name}>{d.name}</span>
        {d.debugMode && (
          <Tooltip title="调试模式">
            <span className="rn-debug"><BugOutlined /></span>
          </Tooltip>
        )}
      </div>
      <div className="rn-type">
        {d.ruleType} · {kindLabel} · {subCount} 子节点
      </div>
      <span className="rn-enter" onClick={(e) => { e.stopPropagation(); onEnterSub?.(id); }}>
        <LoginOutlined /> 进入子画布 ▸
      </span>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

export default memo(ContainerNodeInner);
