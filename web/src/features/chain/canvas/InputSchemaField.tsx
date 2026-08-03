import { useEffect, useMemo, useRef, useState } from 'react';
import { Button, Checkbox, Input, Segmented, Select, Table } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

import JsonField from './JsonField';
import {
  PARAM_TYPES,
  emptyRow,
  rowsToSchema,
  schemaToRows,
  type ParamRow,
} from './schemaParams';

export interface InputSchemaFieldProps {
  value?: Record<string, unknown>;
  onChange?: (v: Record<string, unknown> | undefined) => void;
}

const TYPE_OPTIONS = PARAM_TYPES.map((t) => ({ value: t, label: t }));

const TYPE_PLACEHOLDER: Record<string, string> = {
  string: '文本',
  number: '如 35',
  integer: '如 3',
  boolean: 'true / false',
  object: '{"k":1}',
  array: '[1,2]',
};

// Apifox 式入参编辑器：结构化参数表格 ↔ JSON Schema 双视图实时同步。
// 受控：内部维护 ParamRow[]，向上输出 JSON Schema（全空 → undefined）。
// 「描述」列映射为标准 description 关键字；JSON 源码视图支持 // 注释（保存前剥离，见 JsonField/parseJsonc）。
export default function InputSchemaField({ value, onChange }: InputSchemaFieldProps) {
  const [view, setView] = useState<'table' | 'json'>('table');
  const [rows, setRows] = useState<ParamRow[]>(() => schemaToRows(value));
  // 记录最近一次由本组件 onChange 发出的 schema 指纹，用于识别"自身回环"，
  // 避免在受控回环（尤其中文输入法 composition）中重置内部行、打断输入。
  const lastEmittedRef = useRef<string | null>(null);

  // 外部值变化（切换链 / JSON 视图编辑 / MCP 选链预填）时重置内部行。
  // 若新值正是本组件刚发出的（受控回环），跳过重置以保持输入连续性。
  useEffect(() => {
    const incoming = JSON.stringify(value ?? null);
    if (lastEmittedRef.current !== null && incoming === lastEmittedRef.current) {
      return; // 自身回环：不重置，避免打断 IME/输入
    }
    lastEmittedRef.current = null;
    setRows(schemaToRows(value));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [JSON.stringify(value ?? null)]);

  const commit = (next: ParamRow[]) => {
    setRows(next);
    const schema = rowsToSchema(next);
    lastEmittedRef.current = JSON.stringify(schema ?? null);
    onChange?.(schema);
  };

  const update = (key: string, patch: Partial<ParamRow>) =>
    commit(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));

  const remove = (key: string) => commit(rows.filter((r) => r.key !== key));

  const add = () => commit([...rows, emptyRow()]);

  const columns: ColumnsType<ParamRow> = useMemo(
    () => [
      {
        title: '参数名',
        dataIndex: 'name',
        width: 150,
        render: (_: unknown, r) => (
          <Input
            size="small"
            value={r.name}
            placeholder="如 t"
            onChange={(e) => update(r.key, { name: e.target.value })}
          />
        ),
      },
      {
        title: '类型',
        dataIndex: 'type',
        width: 108,
        render: (_: unknown, r) => (
          <Select
            size="small"
            style={{ width: '100%' }}
            value={r.type}
            options={TYPE_OPTIONS}
            onChange={(t) => update(r.key, { type: t })}
          />
        ),
      },
      {
        title: '必填',
        dataIndex: 'required',
        width: 52,
        align: 'center',
        render: (_: unknown, r) => (
          <Checkbox
            checked={r.required}
            onChange={(e) => update(r.key, { required: e.target.checked })}
          />
        ),
      },
      {
        title: '默认值',
        dataIndex: 'default',
        width: 130,
        render: (_: unknown, r) => (
          <Input
            size="small"
            value={r.default}
            placeholder={TYPE_PLACEHOLDER[r.type]}
            onChange={(e) => update(r.key, { default: e.target.value })}
          />
        ),
      },
      {
        title: '描述（注释）',
        dataIndex: 'description',
        render: (_: unknown, r) => (
          <Input
            size="small"
            value={r.description}
            placeholder="参数含义，如：温度（℃）"
            onChange={(e) => update(r.key, { description: e.target.value })}
          />
        ),
      },
      {
        title: '',
        key: 'op',
        width: 40,
        render: (_: unknown, r) => (
          <Button
            danger
            type="text"
            size="small"
            icon={<DeleteOutlined />}
            aria-label={`删除参数 ${r.name || ''}`}
            onClick={() => remove(r.key)}
          />
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rows],
  );

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 8 }}>
        <Segmented
          size="small"
          value={view}
          onChange={(v) => setView(v as 'table' | 'json')}
          options={[
            { value: 'table', label: '参数表格' },
            { value: 'json', label: 'JSON 源码' },
          ]}
        />
      </div>

      {view === 'table' ? (
        <>
          <Table<ParamRow>
            size="small"
            rowKey="key"
            columns={columns}
            dataSource={rows}
            pagination={false}
            locale={{ emptyText: '暂无条件参数，点击下方添加' }}
            style={{ marginBottom: 8 }}
          />
          <Button type="dashed" block size="small" icon={<PlusOutlined />} onClick={add}>
            添加参数
          </Button>
        </>
      ) : (
        <JsonField
          rows={10}
          allowComments
          value={value}
          onChange={(v) => onChange?.(v as Record<string, unknown> | undefined)}
          placeholder='{"type":"object","properties":{"t":{"type":"number","description":"温度"}},"required":["t"]}  // 支持 // 注释'
        />
      )}
    </div>
  );
}
