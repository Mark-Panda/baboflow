import { useMemo, useState } from 'react';
import { Button, Space, Tag, Tooltip } from 'antd';
import {
  CaretRightOutlined,
  CheckCircleFilled,
  ClearOutlined,
  CloseCircleFilled,
  DownOutlined,
  UpOutlined,
} from '@ant-design/icons';

import { NodeTrace } from '@/api/chain';
import CodeField from '@/components/CodeField';
import { relationZhName } from './componentZh';

export interface DebugPanelProps {
  running: boolean;
  output: string;
  error: string;
  traces: NodeTrace[];
  // 画布节点 id -> 显示名（用于把 nodeId 映射成中文节点名）
  nodeNames?: Record<string, string>;
  onRun: (input: string) => void;
  onClear: () => void;
  // 在画布中定位/选中节点
  onLocateNode?: (nodeId: string) => void;
}

export default function DebugPanel({
  running,
  output,
  error,
  traces,
  nodeNames,
  onRun,
  onClear,
  onLocateNode,
}: DebugPanelProps) {
  const [input, setInput] = useState('{}');
  const [open, setOpen] = useState(true);

  const summary = useMemo(() => {
    const failed = traces.filter((t) => t.err).length;
    return { total: traces.length, failed };
  }, [traces]);

  const hasResult = traces.length > 0 || output || error;

  return (
    <div className="bf-debug-console">
      {/* 头部：标题 + 输入 + 操作 */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '6px 12px',
          userSelect: 'none',
        }}
      >
        <span
          style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6 }}
          onClick={() => setOpen((o) => !o)}
        >
          {open ? <DownOutlined style={{ fontSize: 11 }} /> : <UpOutlined style={{ fontSize: 11 }} />}
          <span style={{ fontWeight: 600, fontSize: 13 }}>调试控制台</span>
          {hasResult && (
            <span style={{ fontSize: 11, color: '#a2a9bd' }}>
              {summary.failed > 0 ? (
                <span style={{ color: '#cf1322' }}>{summary.failed} 失败 / </span>
              ) : null}
              {summary.total} 节点
            </span>
          )}
        </span>
        <Space size="small" style={{ marginLeft: 'auto' }}>
          <Tooltip title="清空结果">
            <Button size="small" icon={<ClearOutlined />} onClick={onClear} />
          </Tooltip>
          <Button
            size="small"
            type="primary"
            icon={<CaretRightOutlined />}
            loading={running}
            onClick={() => onRun(input)}
          >
            运行
          </Button>
        </Space>
      </div>

      {open && (
        <div style={{ maxHeight: 320, overflow: 'auto', padding: '0 12px 12px' }}>
          {/* 输入区 */}
          <div style={{ marginBottom: 10 }}>
            <div style={{ fontSize: 12, color: '#6b7280', marginBottom: 4 }}>
              输入消息（JSON）
            </div>
            <CodeField
              language="json"
              rows={3}
              value={input}
              onChange={setInput}
              placeholder='输入 JSON，如 {"t":35}'
            />
          </div>

          {/* 错误置顶 */}
          {error && (
            <div
              style={{
                padding: '6px 10px',
                background: '#fff1f0',
                border: '1px solid #ffccc7',
                borderRadius: 6,
                color: '#cf1322',
                fontSize: 12,
                marginBottom: 8,
                whiteSpace: 'pre-wrap',
              }}
            >
              ✘ {error}
            </div>
          )}

          {/* 节点轨迹时间线 */}
          {traces.length > 0 && (
            <div style={{ marginBottom: 8 }}>
              {traces.map((t, i) => (
                <TraceRow
                  key={i}
                  trace={t}
                  name={nodeNames?.[t.nodeId] ?? t.nodeId}
                  onLocate={() => onLocateNode?.(t.nodeId)}
                />
              ))}
            </div>
          )}

          {/* 最终输出 */}
          {output && (
            <div>
              <div style={{ fontSize: 12, fontWeight: 600, color: '#3fbf6b', marginBottom: 4 }}>
                最终输出
              </div>
              <ReadonlyJson value={output} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function TraceRow({
  trace: t,
  name,
  onLocate,
}: {
  trace: NodeTrace;
  name: string;
  onLocate: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const failed = !!t.err;
  return (
    <div
      style={{
        border: '1px solid #eef0f5',
        borderLeft: `3px solid ${failed ? '#f5222d' : '#3fbf6b'}`,
        borderRadius: 6,
        marginBottom: 6,
        background: '#fff',
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '6px 10px',
          cursor: 'pointer',
        }}
        onClick={() => setExpanded((e) => !e)}
      >
        {failed ? (
          <CloseCircleFilled style={{ color: '#f5222d' }} />
        ) : (
          <CheckCircleFilled style={{ color: '#3fbf6b' }} />
        )}
        <span style={{ fontWeight: 600, fontSize: 12 }}>{name}</span>
        <span style={{ fontFamily: 'monospace', fontSize: 11, color: '#a2a9bd' }}>
          {t.nodeId}
        </span>
        <span style={{ marginLeft: 'auto', display: 'flex', gap: 6, alignItems: 'center' }}>
          {typeof t.durationMs === 'number' && (
            <Tag style={{ marginRight: 0 }}>{t.durationMs} ms</Tag>
          )}
          {(t.relationType || t.flowType) && (
            <Tag color={failed ? 'error' : 'default'} style={{ marginRight: 0 }}>
              {t.relationType ? relationZhName(t.relationType) : t.flowType}
            </Tag>
          )}
          <Tooltip title="在画布中定位">
            <Button
              size="small"
              type="text"
              style={{ fontSize: 11, padding: '0 4px' }}
              onClick={(e) => {
                e.stopPropagation();
                onLocate();
              }}
            >
              定位
            </Button>
          </Tooltip>
          {expanded ? (
            <UpOutlined style={{ fontSize: 10, color: '#a2a9bd' }} />
          ) : (
            <DownOutlined style={{ fontSize: 10, color: '#a2a9bd' }} />
          )}
        </span>
      </div>

      {expanded && (
        <div style={{ padding: '0 10px 10px', display: 'grid', gap: 8 }}>
          {t.err && (
            <div
              style={{
                padding: '4px 8px',
                background: '#fff1f0',
                borderRadius: 4,
                color: '#cf1322',
                fontSize: 12,
                whiteSpace: 'pre-wrap',
              }}
            >
              {t.err}
            </div>
          )}
          <div>
            <div style={{ fontSize: 11, color: '#6b7280', marginBottom: 2 }}>输入</div>
            <ReadonlyJson value={t.in ?? ''} />
          </div>
          <div>
            <div style={{ fontSize: 11, color: '#6b7280', marginBottom: 2 }}>输出</div>
            <ReadonlyJson value={t.out ?? t.data ?? ''} />
          </div>
        </div>
      )}
    </div>
  );
}

// 只读 JSON 展示（美化 + 等宽深色块）。
function ReadonlyJson({ value }: { value: string }) {
  return (
    <pre
      style={{
        margin: 0,
        fontSize: 11,
        background: '#0f1420',
        color: '#d6e2ff',
        padding: 10,
        borderRadius: 6,
        overflow: 'auto',
        maxHeight: 180,
      }}
    >
      {formatMaybeJson(value)}
    </pre>
  );
}

function formatMaybeJson(s: string): string {
  if (!s) return '(空)';
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}
