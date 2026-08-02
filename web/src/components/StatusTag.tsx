import { Tag } from 'antd';

const STATUS: Record<string, { color: string; label: string }> = {
  draft: { color: 'default', label: '草稿' },
  published: { color: 'green', label: '已发布' },
  archived: { color: 'orange', label: '已归档' },
  running: { color: 'processing', label: '运行中' },
  pending: { color: 'default', label: '待办' },
  success: { color: 'success', label: '成功' },
  failure: { color: 'error', label: '失败' },
  timeout: { color: 'warning', label: '超时' },
};

export default function StatusTag({ value }: { value: string }) {
  const s = STATUS[value] || { color: 'default', label: value };
  return <Tag color={s.color}>{s.label}</Tag>;
}
