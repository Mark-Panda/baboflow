import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  App, Card, Drawer, Select, Space, Table, Tag, Button, Tabs, Empty, Descriptions,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { ReactFlow, Background, Controls, ReactFlowProvider, MarkerType } from '@xyflow/react';

import { runApi, ChainRun } from '@/api/run';
import { chainApi, ChainListItem, TraceMessage } from '@/api/chain';
import { toSafeNumber } from '@/api/http';
import { componentApi } from '@/api/component';
import StatusTag from '@/components/StatusTag';
import { dslToFlow, EDGE_TYPE, DslChain } from '../chain/canvas/chainDsl';
import { layoutFlowElk } from '../chain/canvas/elkLayout';
import RuleNode from '../chain/canvas/nodes/RuleNode';
import ContainerNode from '../chain/canvas/nodes/ContainerNode';
import AvoidEdge from '../chain/canvas/edges/AvoidEdge';
import '../chain/canvas/canvas.css';
import '@xyflow/react/dist/style.css';

const nodeTypes = { rule: RuleNode, container: ContainerNode };
const edgeTypes = { [EDGE_TYPE]: AvoidEdge };
const TRIGGER_LABEL: Record<string, string> = { manual: '手动', task: '看板', mcp: 'MCP', cron: '定时' };

export default function RunLogPage() {
  const { message } = App.useApp();
  const [data, setData] = useState<ChainRun[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [chains, setChains] = useState<ChainListItem[]>([]);
  const [chainId, setChainId] = useState<string | undefined>();
  const [status, setStatus] = useState<string | undefined>();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [detail, setDetail] = useState<ChainRun | null>(null);

  const chainName = useMemo(() => {
    const m = new Map(chains.map((c) => [c.id, c.name]));
    return (id: string) => m.get(id) ?? id;
  }, [chains]);

  useEffect(() => {
    chainApi.list({ page: 1, pageSize: 200 }).then((r) => setChains(r.list || [])).catch(() => {});
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await runApi.list({ chainId, status, page, pageSize });
      setData(res.list || []);
      setTotal(res.page ? toSafeNumber(res.page.total, 'rule chain run total') : 0);
    } catch {
      message.error('加载运行日志失败');
    } finally {
      setLoading(false);
    }
  }, [chainId, status, page, pageSize, message]);

  useEffect(() => { load(); }, [load]);

  const openDetail = async (r: ChainRun) => {
    try {
      const d = await runApi.detail(r.id);
      setDetail(d);
    } catch {
      setDetail(r);
    }
  };

  const columns: ColumnsType<ChainRun> = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: '规则链', dataIndex: 'chainId', render: (v) => chainName(v) },
    { title: '触发', dataIndex: 'trigger', width: 80, render: (v) => TRIGGER_LABEL[v] ?? v },
    { title: '状态', dataIndex: 'status', width: 100, render: (v) => <StatusTag value={v} /> },
    { title: '开始时间', dataIndex: 'startedAt', width: 170, render: (v) => dayjs(v).format('MM-DD HH:mm:ss') },
    {
      title: '耗时', key: 'cost', width: 90,
      render: (_, r) => (r.finishedAt ? `${dayjs(r.finishedAt).diff(dayjs(r.startedAt))}ms` : '—'),
    },
    { title: '错误', dataIndex: 'error', ellipsis: true, render: (v) => (v ? <span style={{ color: '#cf1322' }}>{v}</span> : '—') },
    { title: '操作', key: 'op', width: 90, render: (_, r) => <Button size="small" onClick={() => openDetail(r)}>回放</Button> },
  ];

  return (
    <div className="bf-page">
      <Card
        title="规则链运行日志"
        extra={
          <Space>
            <Select
              allowClear placeholder="规则链" style={{ width: 180 }}
              value={chainId} onChange={(v) => { setChainId(v); setPage(1); }}
              options={chains.map((c) => ({ label: c.name, value: c.id }))}
              showSearch optionFilterProp="label"
            />
            <Select
              allowClear placeholder="状态" style={{ width: 110 }}
              value={status} onChange={(v) => { setStatus(v); setPage(1); }}
              options={['success', 'failure', 'running', 'timeout'].map((s) => ({ label: s, value: s }))}
            />
            <Button icon={<ReloadOutlined />} onClick={load} />
          </Space>
        }
      >
        <Table
          rowKey="id" size="middle" loading={loading} columns={columns} dataSource={data}
          pagination={{
            current: page, pageSize, total, showSizeChanger: true, showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
        />
      </Card>

      <RunDetailDrawer run={detail} chainName={chainName} onClose={() => setDetail(null)} />
    </div>
  );
}

// ---- 回放详情 Drawer ----
function RunDetailDrawer({ run, chainName, onClose }: { run: ChainRun | null; chainName: (id: string) => string; onClose: () => void }) {
  const [flowNodes, setFlowNodes] = useState<ReturnType<typeof dslToFlow>['nodes']>([]);
  const [flowEdges, setFlowEdges] = useState<ReturnType<typeof dslToFlow>['edges']>([]);

  useEffect(() => {
    if (!run) return;
    Promise.all([
      chainApi.get(run.chainId),
      componentApi.list().catch(() => ({ list: [] })),
    ]).then(([c, componentData]) => {
      const { nodes, edges } = dslToFlow((c.dsl as DslChain) ?? {}, componentData.list ?? []);
      // 按 trace 结果着色
      const stateMap = new Map<string, string>();
      (run.nodeTrace ?? []).forEach((t) => stateMap.set(t.nodeId, t.err ? 'failure' : 'success'));
      const colored = nodes.map((n) => ({
        ...n,
        data: { ...n.data, __replay: true },
        className: stateMap.has(n.id) ? `st-${stateMap.get(n.id)}` : 'st-skipped',
      }));
      setFlowEdges(edges.map((e) => ({ ...e, type: EDGE_TYPE, markerEnd: { type: MarkerType.ArrowClosed } })));
      layoutFlowElk(colored, edges).then(setFlowNodes).catch(() => setFlowNodes(colored));
    }).catch(() => { setFlowNodes([]); setFlowEdges([]); });
  }, [run]);

  if (!run) return null;

  const replayCanvas = (
    <div style={{ height: 380, border: '1px solid #eef0f5', borderRadius: 8, overflow: 'hidden' }}>
      {flowNodes.length === 0 ? (
        <Empty description="无画布数据" style={{ marginTop: 120 }} />
      ) : (
        <ReactFlowProvider>
          <ReactFlow
            nodes={flowNodes} edges={flowEdges} nodeTypes={nodeTypes} edgeTypes={edgeTypes}
            fitView nodesDraggable={false} nodesConnectable={false} elementsSelectable={false}
            zoomOnScroll panOnDrag proOptions={{ hideAttribution: true }}
          >
            <Background gap={16} size={1} color="#e8ebf3" />
            <Controls showInteractive={false} />
          </ReactFlow>
        </ReactFlowProvider>
      )}
    </div>
  );

  const timeline = (
    <div style={{ maxHeight: 380, overflow: 'auto' }}>
      {(run.nodeTrace ?? []).length === 0 ? (
        <Empty description="无节点事件" />
      ) : (
        (run.nodeTrace ?? []).map((t, i) => (
          <div key={i} style={{ padding: '8px 10px', borderBottom: '1px solid #f2f3f7' }}>
            <Space>
              <Tag color={t.err ? 'error' : 'success'}>{t.err ? '✘' : '✔'}</Tag>
              <span style={{ fontFamily: 'monospace', fontWeight: 600 }}>{t.nodeId}</span>
              <span style={{ color: '#a2a9bd', fontSize: 12 }}>{t.relationType || t.flowType}</span>
            </Space>
            {t.err && <div style={{ color: '#cf1322', fontSize: 12, marginTop: 4 }}>{t.err}</div>}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8, marginTop: 6 }}>
              <div>
                <div style={{ fontSize: 11, color: '#6b7280', marginBottom: 3 }}>输入</div>
                <pre style={{ margin: 0, fontSize: 11, background: '#0f1420', color: '#d6e2ff', padding: 8, borderRadius: 6, overflow: 'auto', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
                  {fmtTraceMessage(t.input, t.in ?? t.data)}
                </pre>
              </div>
              <div>
                <div style={{ fontSize: 11, color: '#6b7280', marginBottom: 3 }}>输出</div>
                <pre style={{ margin: 0, fontSize: 11, background: '#0f1420', color: '#d6e2ff', padding: 8, borderRadius: 6, overflow: 'auto', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
                  {fmtTraceMessage(t.output, t.out ?? t.data)}
                </pre>
              </div>
            </div>
          </div>
        ))
      )}
    </div>
  );

  return (
    <Drawer
      title={`运行 #${run.id} · ${chainName(run.chainId)}`}
      open={!!run} onClose={onClose} width={860}
    >
      <Descriptions column={3} size="small" style={{ marginBottom: 16 }}>
        <Descriptions.Item label="状态"><StatusTag value={run.status} /></Descriptions.Item>
        <Descriptions.Item label="触发">{TRIGGER_LABEL[run.trigger] ?? run.trigger}</Descriptions.Item>
        <Descriptions.Item label="耗时">{run.finishedAt ? `${dayjs(run.finishedAt).diff(dayjs(run.startedAt))}ms` : '—'}</Descriptions.Item>
      </Descriptions>
      {run.error && (
        <div style={{ padding: '8px 12px', background: '#fff1f0', border: '1px solid #ffccc7', borderRadius: 6, color: '#cf1322', fontSize: 13, marginBottom: 16 }}>
          {run.error}
        </div>
      )}
      <Tabs
        items={[
          { key: 'replay', label: '画布回放', children: replayCanvas },
          { key: 'timeline', label: '节点时间线', children: timeline },
          {
            key: 'io', label: '输入 / 输出',
            children: (
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <div>
                  <div style={{ fontWeight: 600, marginBottom: 6 }}>输入</div>
                  <pre style={{ fontSize: 11, background: '#0f1420', color: '#d6e2ff', padding: 12, borderRadius: 6, overflow: 'auto' }}>{fmtJson(JSON.stringify(run.input))}</pre>
                </div>
                <div>
                  <div style={{ fontWeight: 600, marginBottom: 6 }}>输出</div>
                  <pre style={{ fontSize: 11, background: '#0f1420', color: '#d6e2ff', padding: 12, borderRadius: 6, overflow: 'auto' }}>{fmtJson(JSON.stringify(run.output))}</pre>
                </div>
              </div>
            ),
          },
        ]}
      />
    </Drawer>
  );
}

function fmtJson(s: string): string {
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s || '(空)'; }
}

function fmtTraceMessage(message: TraceMessage | undefined, fallback: string): string {
  if (!message) return fmtJson(fallback);
  return JSON.stringify({
    msg: parseJson(message.msg),
    metadata: message.metadata,
    type: message.type,
    dataType: message.dataType,
  }, null, 2);
}

function parseJson(s: string): unknown {
  try { return JSON.parse(s); } catch { return s || '(空)'; }
}
