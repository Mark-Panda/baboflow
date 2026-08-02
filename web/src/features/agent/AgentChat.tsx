import { useEffect, useRef, useState } from 'react';
import {
  Avatar,
  Button,
  Empty,
  Input,
  List,
  Popconfirm,
  Spin,
  Tag,
  Tooltip,
  Upload,
  message as antdMessage,
} from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  SendOutlined,
  PaperClipOutlined,
  RobotOutlined,
  UserOutlined,
  ToolOutlined,
  LoadingOutlined,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';

import * as api from '@/api/agent';
import { useChatStore } from '@/stores/chatStore';
import type { ChatMsg } from '@/stores/chatStore';
import './agentChat.css';

interface AgentChatProps {
  agent: api.Agent;
}

// Agent 对话：左侧会话列表 + 右侧消息流 + 底部输入（支持文件/图片）。
export default function AgentChat({ agent }: AgentChatProps) {
  const [sessions, setSessions] = useState<api.AgentSession[]>([]);
  const [loadingSessions, setLoadingSessions] = useState(false);
  const { sessionId, messages, sending, openSession, send, reset } = useChatStore();
  const [draft, setDraft] = useState('');
  const [pendingFiles, setPendingFiles] = useState<api.Asset[]>([]);
  const [uploading, setUploading] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  const refreshSessions = async () => {
    setLoadingSessions(true);
    try {
      const { list } = await api.listSessions(agent.key);
      setSessions(list);
    } finally {
      setLoadingSessions(false);
    }
  };

  useEffect(() => {
    reset();
    refreshSessions();
    return () => reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agent.key]);

  // 消息到底部
  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [messages]);

  const newSession = async () => {
    const s = await api.createSession(agent.key);
    await refreshSessions();
    await openSession(s.id);
  };

  const removeSession = async (id: string) => {
    await api.deleteSession(id);
    if (id === sessionId) reset();
    await refreshSessions();
  };

  const doSend = () => {
    const text = draft.trim();
    if (!text || !sessionId || sending) return;
    send(agent.key, text, pendingFiles.map((f) => f.id));
    setDraft('');
    setPendingFiles([]);
  };

  const doUpload = async (file: File) => {
    if (!sessionId) {
      antdMessage.warning('请先选择或新建会话');
      return false;
    }
    setUploading(true);
    try {
      const asset = await api.uploadAsset(sessionId, file);
      setPendingFiles((prev) => [...prev, asset]);
      antdMessage.success(`${file.name} 已就绪`);
    } catch {
      // http 拦截器已提示
    } finally {
      setUploading(false);
    }
    return false; // 阻止默认上传
  };

  return (
    <div className="agent-chat">
      {/* 会话列表 */}
      <div className="chat-sessions">
        <Button block type="primary" icon={<PlusOutlined />} onClick={newSession} style={{ marginBottom: 12 }}>
          新会话
        </Button>
        <Spin spinning={loadingSessions}>
          {sessions.length === 0 ? (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无会话" />
          ) : (
            <List
              size="small"
              dataSource={sessions}
              renderItem={(s) => (
                <List.Item
                  className={`session-item ${s.id === sessionId ? 'active' : ''}`}
                  onClick={() => openSession(s.id)}
                  actions={[
                    <Popconfirm key="del" title="删除该会话？" onConfirm={() => removeSession(s.id)}>
                      <DeleteOutlined className="session-del" onClick={(e) => e.stopPropagation()} />
                    </Popconfirm>,
                  ]}
                >
                  <span className="session-title">{s.title}</span>
                </List.Item>
              )}
            />
          )}
        </Spin>
      </div>

      {/* 消息区 */}
      <div className="chat-main">
        <div className="chat-messages" ref={scrollRef}>
          {!sessionId ? (
            <div className="chat-empty">
              <RobotOutlined style={{ fontSize: 40, color: '#c0c4cc' }} />
              <p>选择左侧会话，或新建会话开始与「{agent.name}」对话</p>
            </div>
          ) : messages.length === 0 ? (
            <div className="chat-empty">
              <p>向「{agent.name}」提问吧</p>
            </div>
          ) : (
            messages.map((m) => <MessageBubble key={m.id} msg={m} />)
          )}
          {sending && messages[messages.length - 1]?.streaming && (
            <div className="chat-typing">
              <LoadingOutlined /> 思考中…
            </div>
          )}
        </div>

        {/* 待发送附件 */}
        {pendingFiles.length > 0 && (
          <div className="chat-attachments">
            {pendingFiles.map((f) => (
              <Tag key={f.id} closable onClose={() => setPendingFiles((p) => p.filter((x) => x.id !== f.id))}>
                <PaperClipOutlined /> {f.name}
              </Tag>
            ))}
          </div>
        )}

        {/* 输入区 */}
        <div className="chat-input">
          <Upload showUploadList={false} beforeUpload={(f) => doUpload(f)} accept="image/*,.txt,.md,.json,.csv,.pdf">
            <Button icon={<PaperClipOutlined spin={uploading} />} disabled={!sessionId} />
          </Upload>
          <Input.TextArea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={sessionId ? '输入消息，Enter 发送，Shift+Enter 换行' : '请先新建/选择会话'}
            autoSize={{ minRows: 1, maxRows: 6 }}
            disabled={!sessionId}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                doSend();
              }
            }}
          />
          <Button type="primary" icon={<SendOutlined />} onClick={doSend} disabled={!sessionId || !draft.trim() || sending} />
        </div>
      </div>
    </div>
  );
}

function MessageBubble({ msg }: { msg: ChatMsg }) {
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
      </div>
    </div>
  );
}
