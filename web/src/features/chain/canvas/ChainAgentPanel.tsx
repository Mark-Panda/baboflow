import { useEffect, useRef, useState } from 'react';
import { Button, Input, Spin, Tooltip } from 'antd';
import { CloseOutlined, LoadingOutlined, PlusOutlined, RobotOutlined, SendOutlined } from '@ant-design/icons';

import * as api from '@/api/agent';
import { useCanvasChatStore } from '@/stores/canvasChatStore';
import MessageBubble from '@/features/agent/MessageBubble';
import '@/features/agent/agentChat.css';

const AGENT_KEY = 'agent-chain-builder';

interface ChainAgentPanelProps {
  open: boolean;
  onClose: () => void;
  chainId: string;
  // 发送前取当前画布 DSL（flowToDsl 序列化结果），由编辑器注入，供增量编辑。
  getCanvasDsl: () => string;
}

// 画布内嵌「规则链生成器」对话面板：与节点配置、调试控制台共享右侧区域。
// 复用 MessageBubble 渲染消息与工具调用；发送时携带当前画布 DSL，Agent 用 ReAct
// 检索组件→校验→apply_chain_dsl 回传，DSL 经 canvasChatStore 回调应用到画布。
export default function ChainAgentPanel({ open, onClose, chainId, getCanvasDsl }: ChainAgentPanelProps) {
  const { sessionId, messages, sending, openSession, newSession, send, answerQuestion } = useCanvasChatStore();
  const [draft, setDraft] = useState('');
  const [creating, setCreating] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  // 首次打开时若没有会话则新建一个（标题留空，首条消息后自动命名）。
  useEffect(() => {
    if (open && !sessionId && !creating) {
      setCreating(true);
      (async () => {
        try {
          const { list } = await api.listSessions(AGENT_KEY);
          const existing = chainId
            ? list.find((session) => session.chainId === chainId)
            : undefined;
          if (existing) {
            await openSession(existing.id);
          } else {
            await newSession(AGENT_KEY, undefined, chainId);
          }
        } finally {
          setCreating(false);
        }
      })();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chainId, open, sessionId]);

  // 消息到底部
  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [messages]);

  const doSend = () => {
    const text = draft.trim();
    if (!text || !sessionId || sending) return;
    send(AGENT_KEY, text, getCanvasDsl());
    setDraft('');
  };

  const handleQuestionAnswer = (questionId: string, answer: string) => {
    if (!sessionId || sending) return;
    answerQuestion(questionId, answer);
    send(AGENT_KEY, answer, getCanvasDsl());
  };

  const handleNewSession = async () => {
    setCreating(true);
    try {
      await newSession(AGENT_KEY, undefined, chainId);
    } finally {
      setCreating(false);
    }
  };

  if (!open) return null;

  return (
    <div
      className="chain-agent-panel"
      style={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          minHeight: 47,
          padding: '0 12px',
          borderBottom: '1px solid #f0f0f0',
        }}
      >
        <span style={{ fontWeight: 600 }}>
          <RobotOutlined style={{ marginRight: 8, color: '#722ed1' }} />
          规则链生成器
        </span>
        <div style={{ marginLeft: 'auto', display: 'flex', gap: 4 }}>
          <Tooltip title="新建会话">
            <Button size="small" icon={<PlusOutlined />} loading={creating} onClick={handleNewSession} />
          </Tooltip>
          <Tooltip title="关闭">
            <Button size="small" type="text" icon={<CloseOutlined />} onClick={onClose} />
          </Tooltip>
        </div>
      </div>
      <div className="chat-main" style={{ flex: 1, minHeight: 0 }}>
        <div className="chat-messages" ref={scrollRef}>
          <Spin spinning={creating && !sessionId}>
            {!sessionId ? (
              <div className="chat-empty">
                <RobotOutlined style={{ fontSize: 40, color: '#c0c4cc' }} />
                <p>正在准备会话…</p>
              </div>
            ) : messages.length === 0 ? (
              <div className="chat-empty">
                <RobotOutlined style={{ fontSize: 40, color: '#c0c4cc' }} />
                <p>描述你想要的规则链，我会在当前画布生成</p>
                <p style={{ fontSize: 12, color: '#b0b4bc' }}>例如：接收 HTTP 请求，校验参数后写入数据库</p>
              </div>
            ) : (
              messages.map((m) => (
                <MessageBubble
                  key={m.id}
                  msg={m}
                  onQuestionAnswer={handleQuestionAnswer}
                />
              ))
            )}
          </Spin>
          {sending && messages[messages.length - 1]?.streaming && (
            <div className="chat-typing">
              <LoadingOutlined /> 思考中…
            </div>
          )}
        </div>

        <div className="chat-input">
          <Input.TextArea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={sessionId ? '描述需求，Enter 发送，Shift+Enter 换行' : '会话准备中…'}
            autoSize={{ minRows: 1, maxRows: 6 }}
            disabled={!sessionId}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                doSend();
              }
            }}
          />
          <Button
            type="primary"
            icon={<SendOutlined />}
            onClick={doSend}
            disabled={!sessionId || !draft.trim() || sending}
          />
        </div>
      </div>
    </div>
  );
}
