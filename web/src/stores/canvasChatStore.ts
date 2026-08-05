import { create } from 'zustand';
import { wsClient } from '@/ws/client';
import type { WsFrame } from '@/ws/types';
import * as api from '@/api/agent';
import type { ChatMsg } from './chatStore';

// 画布内嵌「规则链生成器」对话状态。独立于 useChatStore：
// 现有 useChatStore 用模块级单订阅，编辑器内嵌面板若复用会与 AgentChat 页抢订阅，
// 故单独一份 store，复用 wsClient 单例，并新增 chain_dsl 帧处理与"应用到画布"回调。
interface CanvasChatState {
  sessionId: string | null;
  messages: ChatMsg[];
  sending: boolean;
  open: boolean; // 面板开合
  lastAppliedDsl: string | null;

  openSession: (sessionId: string) => Promise<void>;
  newSession: (agentKey: string, title?: string, chainId?: string) => Promise<void>;
  send: (agentKey: string, content: string, chainDsl?: string) => void;
  answerQuestion: (questionId: string, answer: string) => void;
  setOpen: (open: boolean) => void;
  reset: () => void;
}

let unsubscribe: (() => void) | null = null;
// "应用到画布"回调：由 ChainEditorPage 挂载时注册、卸载时清空（模块级，同 chatStore 的 unsubscribe 模式）。
let applyChainDslCb: ((dsl: string, sessionId: string) => void) | null = null;
let applyChainDslHandlerVersion = 0;

// 注册/清空"应用到画布"回调（在 chain_dsl 帧到达时触发）。
export function setApplyChainDslHandler(
  cb: ((dsl: string, sessionId: string) => void) | null,
): () => void {
  const version = ++applyChainDslHandlerVersion;
  applyChainDslCb = cb;
  return () => {
    if (version === applyChainDslHandlerVersion) {
      applyChainDslCb = null;
    }
  };
}

export const useCanvasChatStore = create<CanvasChatState>((set, get) => ({
  sessionId: null,
  messages: [],
  sending: false,
  open: false,
  lastAppliedDsl: null,

  async openSession(sessionId) {
    if (unsubscribe) {
      unsubscribe();
      unsubscribe = null;
    }
    set({ sessionId, messages: [], sending: false, lastAppliedDsl: null });

    const { list } = await api.listMessages(sessionId);
    const msgs: ChatMsg[] = list.map((m) => ({
      id: m.id,
      role: m.role === 'user' ? 'user' : 'assistant',
      content: m.content,
      toolCalls: m.toolCalls,
      attachment: m.attachment,
      question: m.toolCalls?.find((tool) => tool.question)?.question
        ? {
            ...m.toolCalls.find((tool) => tool.question)!.question!,
            id: m.toolCalls.find((tool) => tool.question)!.questionId || `history-${m.id}`,
            answered: true,
          }
        : undefined,
    }));
    set({ messages: msgs });

    unsubscribe = wsClient.subscribe((frame) => handleFrame(frame, set, get));
    wsClient.send({ action: 'subscribe', channel: 'agent-chat', sessionId });
  },

  async newSession(agentKey, title, chainId) {
    const s = await api.createSession(agentKey, title, chainId);
    await get().openSession(s.id);
  },

  send(agentKey, content, chainDsl) {
    const { sessionId, sending } = get();
    if (!sessionId || sending) return;
    const tempId = `tmp-${Date.now()}`;
    set((s) => ({
      sending: true,
      lastAppliedDsl: null,
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
      chainDsl: chainDsl || undefined,
    });
  },

  answerQuestion(questionId, answer) {
    set((s) => ({
      messages: s.messages.map((message) =>
        message.question?.id === questionId
          ? { ...message, question: { ...message.question, answered: true, answer } }
          : message,
      ),
    }));
  },

  setOpen(open) {
    set({ open });
  },

  reset() {
    if (unsubscribe) {
      unsubscribe();
      unsubscribe = null;
    }
    set({ sessionId: null, messages: [], sending: false, open: false, lastAppliedDsl: null });
  },
}));

function handleFrame(
  frame: WsFrame,
  set: (fn: (s: CanvasChatState) => Partial<CanvasChatState>) => void,
  get: () => CanvasChatState
) {
  if (frame.channel !== 'agent-chat') return;
  const data = frame.data as Record<string, unknown>;
  const sid = data.sessionId as string;
  if (sid !== get().sessionId) return;

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
        const running = calls.map((c, i) => ({ c, i })).filter((x) => x.c.name === tool && !x.c.output).pop();
        if (data.output && running) {
          calls[running.i] = { ...running.c, output: data.output as string };
        } else {
          calls.push(rec);
        }
        msgs[idx] = { ...msgs[idx], toolCalls: calls };
        return { messages: msgs };
      });
      break;
    }
    case 'chain_dsl': {
      // apply_chain_dsl 工具产出完整 DSL：触发"应用到画布"回调。
      // 补一条 running 工具记录，后续 tool_result(done) 帧会找到并填 output，保持单条 Tag。
      const dsl = (data.dsl as string) || '';
      if (!dsl || dsl === get().lastAppliedDsl) break;
      set((s) => {
        const msgs = [...s.messages];
        const idx = findLastStreaming(msgs);
        if (idx >= 0) {
          const calls = [...(msgs[idx].toolCalls || [])];
          calls.push({ name: 'apply_chain_dsl', input: '', status: 'ok' });
          msgs[idx] = { ...msgs[idx], toolCalls: calls };
        }
        return { messages: msgs, lastAppliedDsl: dsl };
      });
      if (applyChainDslCb) applyChainDslCb(dsl, sid);
      break;
    }
    case 'question': {
      const question = {
        id: (data.questionId as string) || `question-${Date.now()}`,
        question: (data.question as string) || '',
        options: Array.isArray(data.options) ? data.options.filter((item): item is string => typeof item === 'string') : [],
        multiple: data.multiple === true,
        allowOther: data.allowOther === true,
      };
      if (!question.question) break;
      set((s) => {
        const msgs = [...s.messages];
        const idx = findLastStreaming(msgs);
        if (idx < 0) return {};
        msgs[idx] = { ...msgs[idx], streaming: false, question };
        return { messages: msgs, sending: false };
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
