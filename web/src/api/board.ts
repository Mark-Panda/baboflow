import http from './http';

// ---- 看板 ----

export interface BoardTask {
  id: number;
  columnId: number;
  title: string;
  payload: string;
  status: 'pending' | 'running' | 'success' | 'failure' | string;
  assignedChainId?: string;
  runId?: number;
  result?: { output?: string; error?: string };
  retryMax: number;
  retryCount: number;
  timeoutSec: number;
  sort: number;
  createdAt: string;
  updatedAt: string;
}

export interface BoardColumn {
  id: number;
  boardId: number;
  name: string;
  sort: number;
  tasks: BoardTask[];
}

export interface Board {
  id: number;
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
  sort?: number;
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
  get: (id: number) => http.get<unknown, BoardDetail>(`/boards/${id}`),
  update: (id: number, in_: BoardInput) => http.put<unknown, unknown>(`/boards/${id}`, in_),
  remove: (id: number) => http.delete<unknown, unknown>(`/boards/${id}`),

  createColumn: (boardId: number, in_: ColumnInput) =>
    http.post<unknown, BoardColumn>(`/boards/${boardId}/columns`, in_),
  updateColumn: (columnId: number, in_: ColumnInput) =>
    http.put<unknown, unknown>(`/boards/columns/${columnId}`, in_),
  removeColumn: (columnId: number) => http.delete<unknown, unknown>(`/boards/columns/${columnId}`),

  createTask: (columnId: number, in_: TaskInput) =>
    http.post<unknown, BoardTask>(`/boards/columns/${columnId}/tasks`, in_),
  updateTask: (id: number, in_: TaskInput) => http.put<unknown, unknown>(`/tasks/${id}`, in_),
  removeTask: (id: number) => http.delete<unknown, unknown>(`/tasks/${id}`),
  moveTask: (id: number, toColumnId: number, toSort: number) =>
    http.post<unknown, unknown>(`/tasks/${id}/move`, { toColumnId, toSort }),
  triggerTask: (id: number) => http.post<unknown, BoardTask>(`/tasks/${id}/trigger`),
};
