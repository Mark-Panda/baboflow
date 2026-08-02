import http from './http';

// RuleGo ComponentForm（后端 configSchema 直接透传）。
export interface ComponentFormField {
  name: string;
  type: string; // string/int/bool/...
  defaultValue?: unknown;
  label?: string;
  desc?: string;
  required?: boolean;
  validate?: string;
  rules?: Array<Record<string, unknown>>;
  fields?: ComponentFormField[];
  component?: { type?: string; language?: string; options?: Array<{ label: string; value: unknown }> } & Record<string, unknown>;
}

export interface ComponentForm {
  type: string;
  category: string;
  fields: ComponentFormField[];
  label?: string;
  desc?: string;
  icon?: string;
  relationTypes?: string[] | null;
  disabled?: boolean;
  componentKind?: string;
}

// /components 返回的列表项
export interface ComponentMeta {
  type: string;
  name: string;
  category: string;
  description: string;
  configSchema: ComponentForm;
  example: Record<string, unknown>;
}

export const componentApi = {
  list: (params?: { category?: string; keyword?: string }) =>
    http.get<unknown, { list: ComponentMeta[] }>('/components', { params }),
};
