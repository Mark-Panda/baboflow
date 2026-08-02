import { create } from 'zustand';
import type { NodeTrace } from '@/api/chain';

// 画布选中节点、调试态、节点状态着色（编辑器内局部使用，跨组件共享）。
export type NodeRunState = 'running' | 'success' | 'failure' | 'skipped';

interface CanvasState {
  selectedNodeId: string | null;
  setSelectedNodeId: (id: string | null) => void;

  // 调试：逐节点状态着色
  nodeStates: Record<string, NodeRunState>;
  setNodeState: (id: string, s: NodeRunState) => void;
  resetNodeStates: () => void;

  // 调试输出（最近一次运行的逐节点事件 + 输出）
  lastRun: { output: string; error: string; traces: NodeTrace[] } | null;
  setLastRun: (r: CanvasState['lastRun']) => void;
}

export const useCanvasStore = create<CanvasState>((set) => ({
  selectedNodeId: null,
  setSelectedNodeId: (id) => set({ selectedNodeId: id }),

  nodeStates: {},
  setNodeState: (id, s) => set((st) => ({ nodeStates: { ...st.nodeStates, [id]: s } })),
  resetNodeStates: () => set({ nodeStates: {} }),

  lastRun: null,
  setLastRun: (r) => set({ lastRun: r }),
}));
