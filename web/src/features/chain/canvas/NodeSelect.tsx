import { useMemo } from 'react';
import { AutoComplete, Select } from 'antd';

import { componentZhName } from './componentZh';

// 当前规则链内可供引用的节点（由 ChainEditorPage 跨子画布打平后传入）。
export interface NodeOption {
  id: string;
  name: string;
  ruleType: string;
}

export interface NodeSelectProps {
  nodes: NodeOption[];
  // 正在配置的节点 id：从可选项里排除自身
  excludeId?: string;
  // true 多选（输出 string[]）；false 单选（输出 string）
  multiple?: boolean;
  // true 允许手输跨链格式（chainId:nodeId / chain:{chainId}）
  freeInput?: boolean;
  value?: unknown;
  onChange?: (v: unknown) => void;
  placeholder?: string;
}

interface Option {
  value: string;
  label: React.ReactNode;
  // 用于过滤的纯文本
  text: string;
}

// 节点选择器：把「填节点 ID」变成可搜索的本链节点下拉（显示中文节点名 + 组件类型）。
// 单选用 AutoComplete（可输可选，支持手输跨链）；多选用 Select tags。
export default function NodeSelect({
  nodes,
  excludeId,
  multiple = false,
  freeInput = false,
  value,
  onChange,
  placeholder,
}: NodeSelectProps) {
  const options = useMemo<Option[]>(
    () =>
      nodes
        .filter((n) => n.id !== excludeId)
        .map((n) => {
          const typeZh = componentZhName(n.ruleType);
          return {
            value: n.id,
            text: `${n.name} ${typeZh} ${n.ruleType}`,
            label: (
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 8,
                }}
              >
                <span
                  style={{
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {n.name}
                </span>
                <span style={{ flex: 'none', fontSize: 11, color: '#a2a9bd' }}>
                  {typeZh}
                </span>
              </div>
            ),
          };
        }),
    [nodes, excludeId],
  );

  // 多选：groupAction/groupFilter.nodeIds，输出 string[]。
  // 兼容旧数据：逗号分隔 string 或 string[]。
  if (multiple) {
    const arr: string[] = Array.isArray(value)
      ? (value as unknown[]).map((x) => String(x))
      : typeof value === 'string' && value.trim() !== ''
        ? value.split(',').map((s) => s.trim()).filter(Boolean)
        : [];
    return (
      <Select
        mode="multiple"
        showSearch
        allowClear
        optionFilterProp="text"
        placeholder={placeholder ?? '请选择节点（可多选）'}
        options={options}
        value={arr}
        onChange={(v) => onChange?.(v)}
        maxTagCount="responsive"
      />
    );
  }

  // 单选：ref/fetchNodeOutput/for/while。AutoComplete 可输可选。
  const str = value == null ? undefined : String(value);
  return (
    <AutoComplete
      showSearch
      allowClear
      optionFilterProp="text"
      placeholder={
        placeholder ??
        (freeInput ? '选择本链节点，或手输 chainId:nodeId' : '请选择节点')
      }
      options={options}
      value={str}
      onChange={(v) => onChange?.(v)}
    />
  );
}
