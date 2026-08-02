import http, { Page } from './http';

// ---- 审计日志 ----

export interface AuditLog {
  id: number;
  userId?: number;
  action: string;
  targetType: string;
  targetId: string;
  detail?: Record<string, unknown>;
  ip: string;
  createdAt: string;
}

export const auditApi = {
  list: (params: { action?: string; userId?: number; page?: number; pageSize?: number }) =>
    http.get<unknown, Page<AuditLog>>('/audit', { params }),
};

// 动作 → 中文标签（与后端 AuditXxx 常量对应）
export const AUDIT_ACTIONS: { value: string; label: string }[] = [
  { value: 'auth.login', label: '登录' },
  { value: 'auth.login_failed', label: '登录失败' },
  { value: 'auth.logout', label: '登出' },
  { value: 'auth.change_password', label: '修改密码' },
  { value: 'llm.create', label: 'LLM 新增' },
  { value: 'llm.update', label: 'LLM 修改' },
  { value: 'llm.delete', label: 'LLM 删除' },
  { value: 'chain.publish', label: '规则链发布' },
  { value: 'chain.offline', label: '规则链撤销' },
  { value: 'chain.delete', label: '规则链删除' },
  { value: 'chain.import', label: '规则链导入' },
  { value: 'mcp.expose', label: 'MCP 暴露' },
  { value: 'mcp.remove_exposure', label: 'MCP 取消暴露' },
  { value: 'skill.delete', label: 'SKILL 删除' },
  { value: 'task.trigger', label: '任务触发' },
  { value: 'cron.trigger', label: '定时触发' },
];

export function auditActionLabel(action: string): string {
  return AUDIT_ACTIONS.find((a) => a.value === action)?.label || action;
}
