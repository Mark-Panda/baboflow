import http from './http';

// ---- SKILL ----

export interface Skill {
  id: number;
  name: string;
  description: string;
  source: 'component' | 'chain' | 'upload' | 'agent' | string;
  chainId?: string;
  frontmatter?: Record<string, unknown>;
  content?: string;
  createdAt: string;
}

export const skillApi = {
  list: (params: { source?: string; keyword?: string }) =>
    http.get<unknown, { list: Skill[] }>('/skills', { params }),
  get: (id: number) => http.get<unknown, Skill>(`/skills/${id}`),
  upload: (content: string, source?: string) =>
    http.post<unknown, Skill>('/skills', { content, source }),
  remove: (id: number) => http.delete<unknown, unknown>(`/skills/${id}`),
  // Agent2：从已发布链反生成 SKILL
  generateFromChain: (chainId: string) =>
    http.post<unknown, Skill>(`/chains/${encodeURIComponent(chainId)}/skill`),
};
