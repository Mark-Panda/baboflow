// WebSocket 消息类型（agent-chat / chain-debug 双 channel）

export type WsChannel = 'agent-chat' | 'chain-debug';

// 客户端 → 服务端
export interface WsSubscribe {
  action: 'subscribe' | 'unsubscribe' | 'input';
  channel: WsChannel;
  // agent-chat
  sessionId?: string;
  agentKey?: string;
  content?: string;
  assetIds?: number[];
  // chain-debug
  chainId?: string;
}

// 服务端 → 客户端：统一帧
export interface WsFrame<T = unknown> {
  channel: WsChannel;
  type: string;
  data: T;
}

// ---- agent-chat ----
export interface AgentDelta {
  sessionId: string;
  messageId: number;
  delta: string;
  done: boolean;
}
export interface AgentToolCall {
  sessionId: string;
  messageId: number;
  tool: string;
  input: string;
  output?: string;
  status: 'running' | 'done' | 'error';
}

// ---- chain-debug ----
export interface ChainDebugEvent {
  chainId: string;
  runId: number;
  nodeId: string;
  flowType: string;
  relationType: string;
  data: string;
  err?: string;
}
