import { memo } from 'react';
import { Handle, Position, NodeProps } from '@xyflow/react';
import { Tooltip } from 'antd';
import { BugOutlined } from '@ant-design/icons';

import { relationTypesForNode, RuleNodeData } from '../chainDsl';
import { catStyle } from '../category';
import { useCanvasStore } from '@/stores/canvasStore';

// 类别兜底推断（节点 data.category 为空时）。
export function inferCat(ruleType: string): string {
  const t = ruleType.toLowerCase();
  if (/(filter|switch|expr)/.test(t)) return 'filter';
  if (/(transform|template)/.test(t)) return 'transform';
  if (/(for|while|fork|join|break|end|group|ref|comment|inclusive)/.test(t)) return 'common';
  if (/(rest|http|mqtt|db|cache|email|ssh|net)/.test(t)) return 'external';
  if (/(delay|exec|log|functions|fetch)/.test(t)) return 'action';
  if (/(^|\/)(flow)$/.test(t)) return 'flow';
  return 'common';
}

function RuleNodeInner({ id, data, selected }: NodeProps) {
  const d = data as RuleNodeData;
  const cat = catStyle(d.category || inferCat(d.ruleType));
  const state = useCanvasStore((s) => s.nodeStates[id]);
  const relationTypes = relationTypesForNode(
    d.ruleType,
    d.configuration,
    [],
    d.relationTypes,
  );

  return (
    <div
      className={`rule-node ${selected ? 'selected' : ''} ${state ? `st-${state}` : ''}`}
      style={{ ['--cat-color' as string]: cat.color }}
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
      <div className="rn-type">{d.ruleType}</div>
      {relationTypes.map((relationType, index) => {
        const top = `${((index + 1) / (relationTypes.length + 1)) * 100}%`;
        return (
          <div className="rn-output" style={{ top }} key={relationType}>
            <Tooltip title={relationType}>
              <span className="rn-output-label">{relationType}</span>
            </Tooltip>
            <Handle
              id={relationType}
              type="source"
              position={Position.Right}
              className={`rn-handle rn-handle-${relationType.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`}
            />
          </div>
        );
      })}
    </div>
  );
}

export default memo(RuleNodeInner);
