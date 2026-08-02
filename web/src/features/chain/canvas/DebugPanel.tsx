import { useState } from 'react';
import { Button, Input, Space, Tag, Tooltip, Collapse } from 'antd';
import { CaretRightOutlined, ClearOutlined, DownOutlined, UpOutlined } from '@ant-design/icons';

import { NodeTrace } from '@/api/chain';

export interface DebugPanelProps {
  running: boolean;
  output: string;
  error: string;
  traces: NodeTrace[];
  onRun: (input: string) => void;
  onClear: () => void;
}

export default function DebugPanel({ running, output, error, traces, onRun, onClear }: DebugPanelProps) {
  const [input, setInput] = useState('{}');
  const [open, setOpen] = useState(true);

  return (
    <div className="bf-debug-console">
      <div
        style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 12px', cursor: 'pointer', userSelect: 'none' }}
        onClick={() => setOpen((o) => !o)}
      >
        {open ? <DownOutlined style={{ fontSize: 11 }} /> : <UpOutlined style={{ fontSize: 11 }} />}
        <span style={{ fontWeight: 600, fontSize: 13 }}>调试控制台</span>
        <Space size="small" style={{ marginLeft: 'auto' }} onClick={(e) => e.stopPropagation()}>
          <Input
            size="small"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder='输入 JSON，如 {"t":35}'
            style={{ width: 280, fontFamily: 'monospace' }}
          />
          <Button size="small" type="primary" icon={<CaretRightOutlined />} loading={running} onClick={() => onRun(input)}>
            运行
          </Button>
          <Tooltip title="清空结果">
            <Button size="small" icon={<ClearOutlined />} onClick={onClear} />
          </Tooltip>
        </Space>
      </div>

      {open && (traces.length > 0 || output || error) && (
        <div style={{ maxHeight: 220, overflow: 'auto', padding: '0 12px 10px' }}>
          {error && (
            <div style={{ padding: '6px 10px', background: '#fff1f0', border: '1px solid #ffccc7', borderRadius: 6, color: '#cf1322', fontSize: 12, marginBottom: 8 }}>
              ✘ {error}
            </div>
          )}
          <Collapse
            ghost
            size="small"
            items={traces.map((t, i) => ({
              key: String(i),
              label: (
                <Space size="small">
                  <Tag color={t.err ? 'error' : 'success'} style={{ marginRight: 0 }}>
                    {t.err ? '✘' : '✔'}
                  </Tag>
                  <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{t.nodeId}</span>
                  <span style={{ color: '#a2a9bd', fontSize: 11 }}>{t.relationType || t.flowType}</span>
                </Space>
              ),
              children: (
                <pre style={{ margin: 0, fontSize: 11, background: '#0f1420', color: '#d6e2ff', padding: 10, borderRadius: 6, overflow: 'auto' }}>
                  {t.err ? `错误: ${t.err}\n\n` : ''}{formatMaybeJson(t.data)}
                </pre>
              ),
            }))}
          />
          {output && (
            <div style={{ marginTop: 8 }}>
              <div style={{ fontSize: 12, fontWeight: 600, color: '#3fbf6b', marginBottom: 4 }}>最终输出</div>
              <pre style={{ margin: 0, fontSize: 11, background: '#0f1420', color: '#d6e2ff', padding: 10, borderRadius: 6, overflow: 'auto' }}>
                {formatMaybeJson(output)}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
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
