import http, { ProtoInt64 } from './http';

// ---- 看板 ----

export interface BoardTask {
  id: ProtoInt64;
  columnId: ProtoInt64;
  title: string;
  payload: string;
  status: 'pending' | 'running' | 'success' | 'failure' | string;
  assignedChainId?: string;
  runId?: ProtoInt64;
  result?: { output?: string; error?: string };
  retryMax: number;
  retryCount: number;
  timeoutSec: number;
  sort: ProtoInt64;
  createdAt: string;
  updatedAt: string;
}

export interface BoardColumn {
  id: ProtoInt64;
  boardId: ProtoInt64;
  name: string;
  sort: ProtoInt64;
  tasks: BoardTask[];
}

export interface Board {
  id: ProtoInt64;
  name: string;
  description: string;
  createdAt: string;
  updatedAt: string;
}

export interface BoardDetail extends Board {
  columns: BoardColumn[];
}

export interface BoardInput {
  name: string;
  description?: string;
}

export interface ColumnInput {
  name: string;
  sort?: ProtoInt64;
}

export interface TaskInput {
  title: string;
  payload?: string;
  assignedChainId?: string;
  retryMax?: number;
  timeoutSec?: number;
}

export const boardApi = {
  list: () => http.get<unknown, { list: Board[] }>('/boards'),
  create: (in_: BoardInput) => http.post<unknown, Board>('/boards', in_),
  get: (id: ProtoInt64) => http.get<unknown, BoardDetail>(`/boards/${id}`),
  update: (id: ProtoInt64, in_: BoardInput) => http.put<unknown, unknown>(`/boards/${id}`, in_),
  remove: (id: ProtoInt64) => http.delete<unknown, unknown>(`/boards/${id}`),

  createColumn: (boardId: ProtoInt64, in_: ColumnInput) =>
    http.post<unknown, BoardColumn>('/board-columns', { boardId, ...in_ }),
  updateColumn: (columnId: ProtoInt64, in_: ColumnInput) =>
    http.put<unknown, unknown>(`/board-columns/${columnId}`, { id: columnId, ...in_ }),
  removeColumn: (columnId: ProtoInt64) => http.delete<unknown, unknown>(`/board-columns/${columnId}`),

  createTask: (columnId: ProtoInt64, in_: TaskInput) =>
    http.post<unknown, BoardTask>('/tasks', { columnId, ...in_ }),
  updateTask: (id: ProtoInt64, in_: TaskInput) => http.put<unknown, unknown>(`/tasks/${id}`, { id, ...in_ }),
  removeTask: (id: ProtoInt64) => http.delete<unknown, unknown>(`/tasks/${id}`),
  moveTask: (id: ProtoInt64, toColumnId: ProtoInt64, toSort: ProtoInt64) =>
    http.post<unknown, unknown>(`/tasks/${id}/move`, { toColumnId, toSort }),
  triggerTask: (id: ProtoInt64) => http.post<unknown, BoardTask>(`/tasks/${id}/trigger`),
};
