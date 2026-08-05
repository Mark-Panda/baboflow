import { useState } from 'react';
import { Button, Checkbox, Input, Radio, Space, Tag } from 'antd';

import type { AgentQuestion } from '@/api/agent';

export interface QuestionCardQuestion extends AgentQuestion {
  id: string;
  answered?: boolean;
  answer?: string;
}

interface QuestionCardProps {
  question: QuestionCardQuestion;
  onAnswer?: (questionId: string, answer: string) => void;
}

export default function QuestionCard({ question, onAnswer }: QuestionCardProps) {
  const [selected, setSelected] = useState<string[]>([]);
  const [other, setOther] = useState('');
  const disabled = question.answered || !onAnswer;

  const submit = () => {
    const values = [...selected, ...(other.trim() ? [other.trim()] : [])];
    if (values.length === 0) return;
    onAnswer?.(question.id, values.join('、'));
  };

  return (
    <div style={{ marginTop: 10, padding: 12, background: '#f7f3ff', border: '1px solid #e4d7ff', borderRadius: 8 }}>
      <div style={{ fontWeight: 600, marginBottom: 8 }}>{question.question}</div>
      {question.multiple ? (
        <Checkbox.Group
          disabled={disabled}
          options={question.options}
          value={selected}
          onChange={(values) => setSelected(values as string[])}
        />
      ) : (
        <Radio.Group
          disabled={disabled}
          value={selected[0]}
          onChange={(event) => setSelected([event.target.value])}
        >
          <Space direction="vertical">
            {question.options.map((option) => <Radio key={option} value={option}>{option}</Radio>)}
          </Space>
        </Radio.Group>
      )}
      {question.allowOther && (
        <Input
          value={other}
          disabled={disabled}
          onChange={(event) => setOther(event.target.value)}
          placeholder="其他说明"
          style={{ marginTop: 10 }}
        />
      )}
      {question.answered ? (
        <div style={{ marginTop: 10 }}>
          <Tag color="purple">已确认：{question.answer || '已提交'}</Tag>
        </div>
      ) : onAnswer ? (
        <Button
          type="primary"
          size="small"
          disabled={selected.length === 0 && !other.trim()}
          onClick={submit}
          style={{ marginTop: 10 }}
        >
          确认选择
        </Button>
      ) : null}
    </div>
  );
}
