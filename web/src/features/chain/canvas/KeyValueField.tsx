import { useEffect, useState } from 'react';
import { Button, Input, Space } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';

export interface KeyValueFieldProps {
  value?: unknown;
  onChange?: (v: Record<string, string>) => void;
  keyPlaceholder?: string;
  valuePlaceholder?: string;
  addText?: string;
}

interface Pair {
  k: string;
  v: string;
}

// 键值对编辑器：用于 headers / env / metadata 等 { [k]: v } 型字段。
// 受控：内部维护行数组，向上输出 Record<string,string>（空键行被过滤）。
export default function KeyValueField({
  value,
  onChange,
  keyPlaceholder = '键',
  valuePlaceholder = '值',
  addText = '添加一行',
}: KeyValueFieldProps) {
  const [pairs, setPairs] = useState<Pair[]>(() => toPairs(value));

  // 外部值变化（如切换节点）时重置本地行。
  useEffect(() => {
    setPairs(toPairs(value));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [JSON.stringify(value ?? {})]);

  const commit = (next: Pair[]) => {
    setPairs(next);
    const obj: Record<string, string> = {};
    next.forEach(({ k, v }) => {
      const key = k.trim();
      if (key) obj[key] = v;
    });
    onChange?.(obj);
  };

  const update = (index: number, patch: Partial<Pair>) =>
    commit(pairs.map((p, i) => (i === index ? { ...p, ...patch } : p)));

  const remove = (index: number) => commit(pairs.filter((_, i) => i !== index));

  const add = () => setPairs([...pairs, { k: '', v: '' }]);

  return (
    <Space direction="vertical" style={{ width: '100%' }} size="small">
      {pairs.map((p, i) => (
        <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 6, width: '100%' }}>
          <Input
            style={{ flex: '0 0 38%', minWidth: 0 }}
            value={p.k}
            placeholder={keyPlaceholder}
            onChange={(e) => update(i, { k: e.target.value })}
          />
          <Input
            style={{ flex: 1, minWidth: 0 }}
            value={p.v}
            placeholder={valuePlaceholder}
            onChange={(e) => update(i, { v: e.target.value })}
          />
          <Button
            danger
            type="text"
            icon={<DeleteOutlined />}
            aria-label={`删除第${i + 1}行`}
            onClick={() => remove(i)}
          />
        </div>
      ))}
      <Button type="dashed" block icon={<PlusOutlined />} onClick={add}>
        {addText}
      </Button>
    </Space>
  );
}

function toPairs(value: unknown): Pair[] {
  if (value == null) return [];
  let obj: Record<string, unknown>;
  if (typeof value === 'string') {
    try {
      obj = JSON.parse(value) as Record<string, unknown>;
    } catch {
      return [];
    }
  } else if (typeof value === 'object' && !Array.isArray(value)) {
    obj = value as Record<string, unknown>;
  } else {
    return [];
  }
  return Object.entries(obj).map(([k, v]) => ({
    k,
    v: typeof v === 'string' ? v : JSON.stringify(v),
  }));
}
