import { create } from 'zustand';
import { wsClient } from '@/ws/client';
import type { WsFrame } from '@/ws/types';
import * as api from '@/api/agent';

// 一条聊天消息（含流式中的临时消息）
export interface ChatMsg {
  id: number | string; // 流式中临时用 string
  role: 'user' | 'assistant';
  content: string;
  toolCalls?: api.ToolCallRec[];
  attachment?: api.AttachmentRef[];
  streaming?: boolean;
}

interface ChatState {
  sessionId: string | null;
  messages: ChatMsg[];
  sending: boolean;
  subscribed: boolean;

  openSession: (sessionId: string) => Promise<void>;
  send: (agentKey: string, content: string, assetIds?: number[]) => void;
  reset: () => void;
}

let unsubscribe: (() => void) | null = null;

export const useChatStore = create<ChatState>((set, get) => ({
  sessionId: null,
  messages: [],
  sending: false,
  subscribed: false,

  async openSession(sessionId) {
    // 清理旧订阅
    if (unsubscribe) {
      unsubscribe();
      unsubscribe = null;
    }
    set({ sessionId, messages: [], sending: false, subscribed: false });

    // 加载历史
    const { list } = await api.listMessages(sessionId);
    const msgs: ChatMsg[] = list.map((m) => ({
      id: m.id,
      role: m.role === 'user' ? 'user' : 'assistant',
      content: m.content,
      toolCalls: m.toolCalls,
      attachment: m.attachment,
    }));
    set({ messages: msgs });

    // 订阅 WS
    unsubscribe = wsClient.subscribe((frame) => handleFrame(frame, set, get));
    wsClient.send({ action: 'subscribe', channel: 'agent-chat', sessionId });
    set({ subscribed: true });
  },

  send(agentKey, content, assetIds) {
    const { sessionId, sending } = get();
    if (!sessionId || sending) return;
    // 乐观插入 user 消息 + 占位的流式 assistant 消息
    const tempId = `tmp-${Date.now()}`;
    set((s) => ({
      sending: true,
      messages: [
        ...s.messages,
        { id: `u-${Date.now()}`, role: 'user', content },
        { id: tempId, role: 'assistant', content: '', streaming: true },
      ],
    }));
    wsClient.send({
      action: 'input',
      channel: 'agent-chat',
      sessionId,
      agentKey,
      content,
      assetIds: assetIds && assetIds.length ? assetIds : undefined,
    });
  },

  reset() {
    if (unsubscribe) {
      unsubscribe();
      unsubscribe = null;
    }
    set({ sessionId: null, messages: [], sending: false, subscribed: false });
  },
}));

function handleFrame(
  frame: WsFrame,
  set: (fn: (s: ChatState) => Partial<ChatState>) => void,
  get: () => ChatState
) {
  if (frame.channel !== 'agent-chat') return;
  const data = frame.data as Record<string, unknown>;
  const sid = data.sessionId as string;
  if (sid !== get().sessionId) return;

  // 找最后一条 streaming 的 assistant 消息（不存在则新建）
  const appendDelta = (delta: string) => {
    set((s) => {
      const msgs = [...s.messages];
      const idx = findLastStreaming(msgs);
      if (idx < 0) return {};
      msgs[idx] = { ...msgs[idx], content: msgs[idx].content + delta };
      return { messages: msgs };
    });
  };

  switch (frame.type) {
    case 'delta': {
      const delta = (data.delta as string) || '';
      const done = data.done as boolean;
      if (done) {
        set((s) => {
          const msgs = [...s.messages];
          const idx = findLastStreaming(msgs);
          if (idx >= 0) msgs[idx] = { ...msgs[idx], streaming: false };
          return { messages: msgs, sending: false };
        });
      } else if (delta) {
        appendDelta(delta);
      }
      break;
    }
    case 'tool_call': {
      const tool = data.tool as string;
      const rec: api.ToolCallRec = {
        name: tool,
        input: (data.input as string) || '',
        output: data.output as string | undefined,
        status: data.status === 'error' ? 'error' : 'ok',
      };
      set((s) => {
        const msgs = [...s.messages];
        const idx = findLastStreaming(msgs);
        if (idx < 0) return {};
        const calls = [...(msgs[idx].toolCalls || [])];
        // 若已有同名 running 记录则更新输出，否则新增
        const running = calls.map((c, i) => ({ c, i })).filter((x) => x.c.name === tool && !x.c.output).pop();
        if (data.output && running) {
          calls[running.i] = { ...running.c, output: data.output as string };
        } else if (!data.output) {
          calls.push(rec);
        } else {
          calls.push(rec);
        }
        msgs[idx] = { ...msgs[idx], toolCalls: calls };
        return { messages: msgs };
      });
      break;
    }
    case 'error': {
      set((s) => {
        const msgs = [...s.messages];
        const idx = findLastStreaming(msgs);
        if (idx >= 0) {
          msgs[idx] = {
            ...msgs[idx],
            streaming: false,
            content: msgs[idx].content || `出错：${(data.err as string) || '未知错误'}`,
          };
        }
        return { messages: msgs, sending: false };
      });
      break;
    }
  }
}

function findLastStreaming(msgs: ChatMsg[]): number {
  for (let i = msgs.length - 1; i >= 0; i--) {
    if (msgs[i].role === 'assistant' && msgs[i].streaming) return i;
  }
  return -1;
}
