import { WsFrame } from './types';

type Handler = (frame: WsFrame) => void;

const MAX_RECONNECT_DELAY = 10000;

// WS 单例：自动重连 + 订阅分发。后端 WS 端点挂在 /ws（同源，cookie 鉴权）。
class WsClient {
  private ws: WebSocket | null = null;
  private handlers = new Set<Handler>();
  private outbox: string[] = [];
  private reconnectDelay = 500;
  private closedByUser = false;
  private url: string;

  constructor() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    this.url = `${proto}://${location.host}/ws`;
  }

  connect() {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    this.closedByUser = false;
    try {
      this.ws = new WebSocket(this.url);
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.ws.onopen = () => {
      this.reconnectDelay = 500;
      // 连接（重连）成功后冲刷缓冲帧，确保 subscribe 等不丢失。
      const pending = this.outbox;
      this.outbox = [];
      pending.forEach((f) => this.ws?.send(f));
    };
    this.ws.onmessage = (ev) => {
      let frame: WsFrame | null = null;
      try {
        frame = JSON.parse(ev.data as string);
      } catch {
        return;
      }
      if (frame) this.handlers.forEach((h) => h(frame!));
    };
    this.ws.onclose = () => {
      this.ws = null;
      if (!this.closedByUser) this.scheduleReconnect();
    };
    this.ws.onerror = () => {
      this.ws?.close();
    };
  }

  private scheduleReconnect() {
    setTimeout(() => {
      if (!this.closedByUser) this.connect();
    }, this.reconnectDelay);
    this.reconnectDelay = Math.min(this.reconnectDelay * 2, MAX_RECONNECT_DELAY);
  }

  send(payload: unknown) {
    const frame = JSON.stringify(payload);
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(frame);
    } else {
      // 连接未就绪（冷启动/重连中）：先缓冲，onopen 时统一冲刷，避免 subscribe/input 静默丢失。
      this.outbox.push(frame);
    }
  }

  subscribe(h: Handler): () => void {
    this.handlers.add(h);
    this.connect();
    return () => this.handlers.delete(h);
  }

  close() {
    this.closedByUser = true;
    this.ws?.close();
    this.ws = null;
  }
}

export const wsClient = new WsClient();
