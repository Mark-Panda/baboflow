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
  hasFiles?: boolean;
  createdAt: string;
}

// 技能包内一个条目（相对技能根）。
export interface SkillFileItem {
  path: string;
  size: number;
  isDir: boolean;
}

export const skillApi = {
  list: (params: { source?: string; keyword?: string }) =>
    http.get<unknown, { list: Skill[] }>('/skills', { params }),
  get: (id: number) => http.get<unknown, Skill>(`/skills/${id}`),
  upload: (content: string, source?: string) =>
    http.post<unknown, Skill>('/skills', { content, source }),
  // 技能包（ZIP 多文件）上传：multipart。
  uploadPackage: (file: File, source?: string) => {
    const fd = new FormData();
    fd.append('file', file);
    if (source) fd.append('source', source);
    return http.post<unknown, Skill>('/skills/package', fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
  },
  // 包文件清单 / 读单个文件 / 下载归档。
  listFiles: (id: number) => http.get<unknown, { list: SkillFileItem[] }>(`/skills/${id}/files`),
  readFile: (id: number, path: string) =>
    http.get<unknown, { path: string; content: string }>(`/skills/${id}/file`, { params: { path } }),
  packageUrl: (id: number) => `/api/v1/skills/${id}/package`,
  remove: (id: number) => http.delete<unknown, unknown>(`/skills/${id}`),
  // Agent2：从已发布链反生成 SKILL
  generateFromChain: (chainId: string) =>
    http.post<unknown, Skill>(`/chains/${encodeURIComponent(chainId)}/skill`),
};
