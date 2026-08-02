import { useEffect, useState } from 'react';

import CodeField from '@/components/CodeField';

export interface JsonFieldProps {
  value?: unknown;
  onChange?: (v: unknown) => void;
  rows?: number;
  placeholder?: string;
}

// JSON 对象/数组编辑器：受控包一层 CodeField(language=json)。
// 本地保存文本，仅当可解析为合法 JSON 时才向上 onChange（非法输入不污染表单）。
export default function JsonField({
  value,
  onChange,
  rows = 6,
  placeholder,
}: JsonFieldProps) {
  const [text, setText] = useState(() => stringify(value));

  // 外部值变化（如切换节点）时重置本地文本。
  useEffect(() => {
    setText(stringify(value));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [JSON.stringify(value)]);

  const handle = (next: string) => {
    setText(next);
    const trimmed = next.trim();
    if (trimmed === '') {
      onChange?.(undefined);
      return;
    }
    try {
      onChange?.(JSON.parse(trimmed));
    } catch {
      // 非法 JSON：只更新本地文本，等待用户修正
    }
  };

  return (
    <CodeField
      language="json"
      rows={rows}
      value={text}
      onChange={handle}
      placeholder={placeholder ?? '请输入 JSON'}
    />
  );
}

function stringify(v: unknown): string {
  if (v == null || v === '') return '';
  if (typeof v === 'string') {
    // 已是字符串：若是 JSON 文本则美化，否则原样
    try {
      return JSON.stringify(JSON.parse(v), null, 2);
    } catch {
      return v;
    }
  }
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return '';
  }
}
