import { useEffect, useState } from 'react';
import { Button, Input, Select } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';

import type { ComponentFormField } from '@/api/component';
import { fieldZh } from './componentZh';
import { staticOptionsFor } from './fieldWidgets';

export interface StructArrayFieldProps {
  // 数组元素的子字段 schema（来自父 field.fields）
  subFields: ComponentFormField[];
  // 组件 ruleType（用于子字段中文 label 与 level 等枚举）
  ruleType: string;
  value?: unknown;
  onChange?: (v: Array<Record<string, unknown>>) => void;
  addText?: string;
}

type Row = { key: number; data: Record<string, unknown> };

let seq = 0;
const nextKey = () => ++seq;

function toRows(value: unknown): Row[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((x) => x && typeof x === 'object' && !Array.isArray(x))
    .map((x) => ({ key: nextKey(), data: { ...(x as Record<string, unknown>) } }));
}

// struct 数组行编辑器：用于 cacheGet/cacheDelete.keys、cacheSet.items。
// 每行按子字段 schema 渲染（level 走枚举下拉，其余文本输入），受控输出对象数组。
export default function StructArrayField({
  subFields,
  ruleType,
  value,
  onChange,
  addText = '添加一行',
}: StructArrayFieldProps) {
  const [rows, setRows] = useState<Row[]>(() => toRows(value));

  // 外部值变化（切换节点/重载 DSL）时重置本地行。
  useEffect(() => {
    setRows(toRows(value));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [JSON.stringify(value ?? [])]);

  const commit = (next: Row[]) => {
    setRows(next);
    onChange?.(next.map((r) => r.data));
  };

  const update = (key: number, field: string, v: unknown) =>
    commit(
      rows.map((r) =>
        r.key === key ? { ...r, data: { ...r.data, [field]: v } } : r,
      ),
    );

  const remove = (key: number) => commit(rows.filter((r) => r.key !== key));

  const add = () => {
    const data: Record<string, unknown> = {};
    subFields.forEach((sf) => {
      // level 默认链级
      if (sf.name === 'level') data[sf.name] = 'chain';
    });
    commit([...rows, { key: nextKey(), data }]);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {rows.map((row) => (
        <div
          key={row.key}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            padding: 6,
            border: '1px solid #eef0f5',
            borderRadius: 6,
            background: '#fff',
          }}
        >
          {subFields.map((sf) => {
            const label = fieldZh(ruleType, sf.name)?.label ?? sf.label ?? sf.name;
            const enumOpts = staticOptionsFor(ruleType, sf.name);
            const v = row.data[sf.name];
            // level 等枚举子字段 → 下拉
            if (enumOpts && enumOpts.length > 0) {
              return (
                <Select
                  key={sf.name}
                  size="small"
                  style={{ flex: '0 0 32%', minWidth: 0 }}
                  options={enumOpts}
                  value={(v as string | number | undefined) ?? undefined}
                  placeholder={label}
                  onChange={(nv) => update(row.key, sf.name, nv)}
                />
              );
            }
            return (
              <Input
                key={sf.name}
                size="small"
                style={{ flex: 1, minWidth: 0 }}
                value={(v as string | undefined) ?? ''}
                placeholder={label}
                onChange={(e) => update(row.key, sf.name, e.target.value)}
              />
            );
          })}
          <Button
            danger
            type="text"
            size="small"
            icon={<DeleteOutlined />}
            aria-label="删除该行"
            onClick={() => remove(row.key)}
          />
        </div>
      ))}
      <Button type="dashed" size="small" icon={<PlusOutlined />} onClick={add}>
        {addText}
      </Button>
    </div>
  );
}
