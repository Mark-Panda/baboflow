// 入参 JSON Schema ↔ Apifox 式参数行 的双向映射 + JSONC 注释剥离。
// 纯函数，便于单测；产物必须是合法 JSON Schema（供 MCP NewToolWithRawSchema 原样暴露）。

export type ParamType = 'string' | 'number' | 'integer' | 'boolean' | 'object' | 'array';

export const PARAM_TYPES: ParamType[] = ['string', 'number', 'integer', 'boolean', 'object', 'array'];

export interface ParamRow {
  /** 行内稳定 id（React key），不进 schema */
  key: string;
  name: string;
  type: ParamType;
  required: boolean;
  /** 文本态默认值；按 type 在 rowsToSchema 里解析 */
  default?: string;
  /** 参数描述/注释 → 映射为标准 JSON Schema 的 description 关键字 */
  description?: string;
}

let rowSeq = 0;
function newRowKey(): string {
  rowSeq += 1;
  return `p_${rowSeq}_${Math.random().toString(36).slice(2, 7)}`;
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function toParamType(v: unknown): ParamType {
  const t = typeof v === 'string' ? v : '';
  return (PARAM_TYPES as string[]).includes(t) ? (t as ParamType) : 'string';
}

// JSON Schema → 参数行。无法解析/非 object/无 properties → 空数组。
export function schemaToRows(schema?: Record<string, unknown>): ParamRow[] {
  if (!isPlainObject(schema)) return [];
  const props = schema.properties;
  if (!isPlainObject(props)) return [];
  const requiredArr = Array.isArray(schema.required) ? schema.required : [];
  const requiredSet = new Set(requiredArr.filter((r): r is string => typeof r === 'string'));
  return Object.entries(props).map(([name, raw]) => {
    const p = isPlainObject(raw) ? raw : {};
    const def = p.default;
    return {
      key: newRowKey(),
      name,
      type: toParamType(p.type),
      required: requiredSet.has(name),
      default:
        def === undefined || def === null
          ? undefined
          : typeof def === 'string'
            ? def
            : JSON.stringify(def),
      description: typeof p.description === 'string' ? p.description : undefined,
    };
  });
}

// 按 type 解析文本默认值为 JSON 值；解析失败回退字符串原值。
function parseDefault(type: ParamType, raw: string): unknown {
  const t = raw.trim();
  if (t === '') return undefined;
  switch (type) {
    case 'number':
    case 'integer': {
      const n = Number(t);
      return Number.isNaN(n) ? raw : n;
    }
    case 'boolean':
      if (t === 'true') return true;
      if (t === 'false') return false;
      return raw;
    case 'object':
    case 'array':
      try {
        return JSON.parse(t);
      } catch {
        return raw;
      }
    default:
      return raw;
  }
}

// 参数行 → JSON Schema。过滤空 name 行；全空 → undefined（前端契约：表示"未声明"）。
export function rowsToSchema(rows: ParamRow[]): Record<string, unknown> | undefined {
  const valid = rows.filter((r) => r.name.trim() !== '');
  if (valid.length === 0) return undefined;
  const properties: Record<string, unknown> = {};
  const required: string[] = [];
  valid.forEach((r) => {
    const name = r.name.trim();
    const prop: Record<string, unknown> = { type: r.type };
    if (r.description && r.description.trim() !== '') prop.description = r.description.trim();
    if (r.default !== undefined && r.default.trim() !== '') {
      prop.default = parseDefault(r.type, r.default);
    }
    properties[name] = prop;
    if (r.required) required.push(name);
  });
  const schema: Record<string, unknown> = { type: 'object', properties };
  if (required.length > 0) schema.required = required;
  return schema;
}

// 构造一个空行。
export function emptyRow(): ParamRow {
  return { key: newRowKey(), name: '', type: 'string', required: false };
}

// ---- JSONC 注释剥离 ----
// 逐字符状态机：剔除 // 与 /* */ 注释，但跳过字符串字面量（含转义引号）内的内容。
// 返回可 JSON.parse 的干净文本；保留字符串与数字原样。
export function stripJsonComments(input: string): string {
  let out = '';
  let inString = false;
  let inLineComment = false;
  let inBlockComment = false;
  for (let i = 0; i < input.length; i += 1) {
    const ch = input[i];
    const next = input[i + 1];

    if (inLineComment) {
      if (ch === '\n') {
        inLineComment = false;
        out += ch;
      }
      continue;
    }
    if (inBlockComment) {
      if (ch === '*' && next === '/') {
        inBlockComment = false;
        i += 1;
      }
      continue;
    }
    if (inString) {
      out += ch;
      if (ch === '\\') {
        // 转义字符：连同下一个字符一起原样输出，避免 \" 误判字符串结束
        if (i + 1 < input.length) {
          out += input[i + 1];
          i += 1;
        }
      } else if (ch === '"') {
        inString = false;
      }
      continue;
    }
    // 非字符串、非注释
    if (ch === '"') {
      inString = true;
      out += ch;
      continue;
    }
    if (ch === '/' && next === '/') {
      inLineComment = true;
      i += 1;
      continue;
    }
    if (ch === '/' && next === '*') {
      inBlockComment = true;
      i += 1;
      continue;
    }
    out += ch;
  }
  return out;
}

// 解析允许注释的 JSON 文本（JSONC）。剥注释后 JSON.parse；失败抛错。
export function parseJsonc(text: string): unknown {
  return JSON.parse(stripJsonComments(text));
}
