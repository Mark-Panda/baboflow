import { Avatar, Tag, Tooltip } from 'antd';
import { RobotOutlined, UserOutlined, ToolOutlined, PaperClipOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';

import * as api from '@/api/agent';
import type { ChatMsg } from '@/stores/chatStore';
import QuestionCard from './QuestionCard';
import './agentChat.css';

// 聊天气泡：user 右对齐纯文本，assistant Markdown + 工具调用 Tag。
// 从 AgentChat 提取为共享组件，供 AgentChat 与画布内嵌 ChainAgentPanel 复用。
export default function MessageBubble({
  msg,
  onQuestionAnswer,
}: {
  msg: ChatMsg;
  onQuestionAnswer?: (questionId: string, answer: string) => void;
}) {
  const isUser = msg.role === 'user';
  return (
    <div className={`msg-row ${isUser ? 'msg-user' : 'msg-assistant'}`}>
      <Avatar
        size={32}
        icon={isUser ? <UserOutlined /> : <RobotOutlined />}
        style={{ background: isUser ? '#1677ff' : '#722ed1', flexShrink: 0 }}
      />
      <div className="msg-body">
        {msg.attachment && msg.attachment.length > 0 && (
          <div className="msg-attachments">
            {msg.attachment.map((a) =>
              a.mime.startsWith('image/') ? (
                <img key={a.assetId} src={api.assetUrl(a.assetId)} alt={a.name} className="msg-img" />
              ) : (
                <Tag key={a.assetId}>
                  <PaperClipOutlined /> {a.name}
                </Tag>
              )
            )}
          </div>
        )}
        <div className={`msg-content ${msg.streaming ? 'streaming' : ''}`}>
          {isUser ? (
            <span>{msg.content}</span>
          ) : msg.content ? (
            <ReactMarkdown>{msg.content}</ReactMarkdown>
          ) : (
            <span className="msg-placeholder">{msg.streaming ? '…' : ''}</span>
          )}
          {msg.streaming && <span className="cursor">▍</span>}
        </div>
        {msg.toolCalls && msg.toolCalls.length > 0 && (
          <div className="msg-tools">
            {msg.toolCalls.map((t, i) => (
              <Tooltip key={i} title={t.output ? `输出: ${t.output.slice(0, 200)}` : t.input?.slice(0, 200)} placement="topLeft">
                <Tag icon={<ToolOutlined />} color="blue">
                  {t.name}
                </Tag>
              </Tooltip>
            ))}
          </div>
        )}
        {msg.question && (
          <QuestionCard question={msg.question} onAnswer={onQuestionAnswer} />
        )}
      </div>
    </div>
  );
}
