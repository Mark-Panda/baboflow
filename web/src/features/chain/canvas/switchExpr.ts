// Switch 分支条件：可视化规则行 ↔ expr-lang 表达式 的双向映射（纯函数，便于单测）。
// RuleGo switch 的 case 是 expr-lang 布尔表达式（变量 msg/metadata/type）。
// 简单模式由「左值 运算符 右值」行生成表达式；无法解析的表达式落入高级模式原样保留。

export type RuleOp =
  | '=='
  | '!='
  | '>'
  | '>='
  | '<'
  | '<='
  | 'contains'
  | 'not contains'
  | 'startsWith'
  | 'endsWith'
  | 'matches'
  | 'empty'
  | 'not empty';

export const RULE_OPS: Array<{ value: RuleOp; label: string; needsValue: boolean }> = [
  { value: '==', label: '等于', needsValue: true },
  { value: '!=', label: '不等于', needsValue: true },
  { value: '>', label: '大于', needsValue: true },
  { value: '>=', label: '大于等于', needsValue: true },
  { value: '<', label: '小于', needsValue: true },
  { value: '<=', label: '小于等于', needsValue: true },
  { value: 'contains', label: '包含', needsValue: true },
  { value: 'not contains', label: '不包含', needsValue: true },
  { value: 'startsWith', label: '开头是', needsValue: true },
  { value: 'endsWith', label: '结尾是', needsValue: true },
  { value: 'matches', label: '正则匹配', needsValue: true },
  { value: 'empty', label: '为空', needsValue: false },
  { value: 'not empty', label: '非空', needsValue: false },
];

export type Combinator = '&&' | '||';

// 一条可视化规则行：left op right?（empty/not empty 无 right）
// join 表示"该条件与前一个条件之间的连接符"（首条忽略）。这允许同一分支内混用 且/或。
export interface CondRule {
  key: string;
  left: string; // 如 msg.temperature / metadata.type / type
  op: RuleOp;
  right?: string; // 文本态右值；生成时按内容智能加引号/转数字
  /** 与前一个条件的连接符；仅从第二条起生效，缺省视为 && */
  join?: Combinator;
}

let condSeq = 0;
export function newCondKey(): string {
  condSeq += 1;
  return `c_${condSeq}_${Math.random().toString(36).slice(2, 7)}`;
}

export function emptyRule(): CondRule {
  return { key: newCondKey(), left: 'msg.', op: '==', right: '' };
}

// ---- 生成：规则行 → expr-lang 片段 ----

// 右值字面量化：数字/布尔/nil 原样；其余加双引号并转义。
function literal(raw: string): string {
  const t = raw.trim();
  if (t === '') return '""';
  if (/^-?\d+(\.\d+)?$/.test(t)) return t;
  if (t === 'true' || t === 'false' || t === 'nil') return t;
  return `"${t.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

export function ruleToExpr(r: CondRule): string {
  const left = r.left.trim() || 'msg';
  switch (r.op) {
    case 'empty':
      return `(${left} == nil || ${left} == "")`;
    case 'not empty':
      return `(${left} != nil && ${left} != "")`;
    case 'contains':
      return `contains(${left}, ${literal(r.right ?? '')})`;
    case 'not contains':
      return `!contains(${left}, ${literal(r.right ?? '')})`;
    case 'startsWith':
      return `startsWith(${left}, ${literal(r.right ?? '')})`;
    case 'endsWith':
      return `endsWith(${left}, ${literal(r.right ?? '')})`;
    case 'matches':
      return `matches(${left}, ${literal(r.right ?? '')})`;
    default:
      return `${left} ${r.op} ${literal(r.right ?? '')}`;
  }
}

// 一组规则行按各自 join 连接为完整表达式。
// 规则行的 join 表示与前一个条件的连接符（首条忽略），支持同一分支混用 且/或。
// 生成是规范的：每条规则都加括号；混用 &&/|| 时，&& 连续段再整体加括号（&& 优先级高于 ||）。
// 这种形态可被 exprToRules 无损解析回带 join 的规则行（往返一致）。
export function rulesToExpr(rules: CondRule[]): string {
  const used = rules.filter((r) => r.left.trim() !== '');
  if (used.length === 0) return '';
  const parts = used.map((r) => ruleToExpr(r));
  if (parts.length === 1) return parts[0];
  const joins = used.map((r, i) => (i === 0 ? undefined : r.join ?? '&&'));
  const distinct = new Set(joins.filter(Boolean));
  // 全为同一连接符 → 逐条加括号拼接。
  if (distinct.size <= 1) {
    const j = (joins[1] ?? '&&') as Combinator;
    return parts.map((p) => `(${p})`).join(` ${j} `);
  }
  // 混用 &&/||：把 && 视为更高优先级，连续 && 段合并为一个与组（整体加括号），遇 || 切分。
  const orGroups: string[][] = [];
  let cur: string[] = [`(${parts[0]})`];
  for (let i = 1; i < parts.length; i += 1) {
    if (joins[i] === '||') {
      orGroups.push(cur);
      cur = [`(${parts[i]})`];
    } else {
      cur.push(`(${parts[i]})`);
    }
  }
  orGroups.push(cur);
  const rendered = orGroups.map((g) => (g.length > 1 ? `(${g.join(' && ')})` : g[0]));
  return rendered.join(' || ');
}

// ---- 解析：expr-lang 表达式 → 规则行（尽力而为；失败返回 null 走高级模式）----

const OP_FUNC: Record<string, RuleOp> = {
  contains: 'contains',
  startsWith: 'startsWith',
  endsWith: 'endsWith',
  matches: 'matches',
};
const OP_SYMBOL: RuleOp[] = ['==', '!=', '>=', '<=', '>', '<'];

// 仅当整串被一对匹配括号包裹时剥掉最外层一对（contains(...) 不被误剥）。
// 最多剥 2 层：混合表达式里与组整体又加了一层括号（如 ((a) && (b)) 的两层），
// 需要剥到内层才能还原与组的 && 连接。
function stripOuterParens(s: string): string {
  for (let layer = 0; layer < 2; layer += 1) {
    if (!s.startsWith('(') || !s.endsWith(')')) return s;
    let depth = 0;
    let inStr = false;
    let wrapsWhole = true;
    for (let i = 0; i < s.length; i += 1) {
      const ch = s[i];
      if (ch === '"' && s[i - 1] !== '\\') inStr = !inStr;
      if (inStr) continue;
      if (ch === '(') depth += 1;
      else if (ch === ')') {
        depth -= 1;
        // 提前闭合说明首括号不包到末尾 → 不是整体包裹
        if (depth === 0 && i < s.length - 1) {
          wrapsWhole = false;
          break;
        }
      }
    }
    if (!wrapsWhole || depth !== 0) return s;
    s = s.slice(1, -1).trim();
  }
  return s;
}

// 解析单个比较/函数调用为一条规则（不含 && / || 组合）；失败返回 null。
// 先剥掉包裹该原子的括号（如 (msg.b > 2)）；&&/|| 组合拆分由 parseConjunction 在更上层完成。
function parseAtom(raw: string): CondRule | null {
  const s = stripOuterParens(raw.trim());
  // empty / not empty
  let m = s.match(/^(.+?)\s*==\s*nil\s*\|\|\s*\1\s*==\s*""$/);
  if (m && !m[1].includes('(')) return { key: newCondKey(), left: m[1].trim(), op: 'empty', join: '&&' };
  m = s.match(/^(.+?)\s*!=\s*nil\s*&&\s*\1\s*!=\s*""$/);
  if (m && !m[1].includes('(')) return { key: newCondKey(), left: m[1].trim(), op: 'not empty', join: '&&' };
  // !contains(...)
  m = s.match(/^!(\w+)\((.+)\)$/);
  if (m && m[1] === 'contains') {
    const args = splitArgs(m[2]);
    if (args.length === 2 && isSimpleOperand(args[0]) && isSimpleOperand(args[1])) {
      return { key: newCondKey(), left: args[0], op: 'not contains', right: unquote(args[1]), join: '&&' };
    }
    return null;
  }
  // func(left, right)
  m = s.match(/^(\w+)\((.+)\)$/);
  if (m && OP_FUNC[m[1]]) {
    const args = splitArgs(m[2]);
    if (args.length === 2 && isSimpleOperand(args[0]) && isSimpleOperand(args[1])) {
      return { key: newCondKey(), left: args[0], op: OP_FUNC[m[1]], right: unquote(args[1]), join: '&&' };
    }
    return null;
  }
  // left <op> right
  for (const op of OP_SYMBOL) {
    const idx = indexOfOp(s, op);
    if (idx > 0) {
      const left = s.slice(0, idx).trim();
      const right = s.slice(idx + op.length).trim();
      // 左值必须是简单操作数（拒绝 len(...) 等函数嵌套，否则重建会改变语义）
      if (left && right && isSimpleOperand(left) && isSimpleOperand(right)) {
        return { key: newCondKey(), left, op, right: unquote(right), join: '&&' };
      }
      return null;
    }
  }
  return null;
}

// 解析一段可能含顶层 && / || 的片段为规则行（各自带 join）；失败返回 null。
// 先尝试把整体当作单个原子（覆盖 empty/not-empty 这类内部自带 || 或 && 的整条规则，
// 如 (msg.v == nil || msg.v == "")）；失败再按顶层连接符拆分（还原与组 (A) && (B)）。
function parseConjunction(expr: string): CondRule[] | null {
  const single = parseAtom(expr);
  if (single) return [single];
  const stripped = stripOuterParens(expr.trim());
  const split = splitTop(stripped);
  if (!split || split.parts.length < 2) return null;
  const out: CondRule[] = [];
  for (let i = 0; i < split.parts.length; i += 1) {
    const atom = parseAtom(split.parts[i]);
    if (!atom) return null;
    out.push({ ...atom, join: i === 0 ? '&&' : split.ops[i - 1] });
  }
  return out;
}

// 简单操作数：变量路径（msg.a.b / metadata.x / type）、数字、布尔、nil、字符串字面量。
// 排除函数调用 / 嵌套表达式 / 运算符，确保能无损往返。
function isSimpleOperand(s: string): boolean {
  const t = s.trim();
  if (t === '') return false;
  if (/^[A-Za-z_][\w.]*$/.test(t)) return true; // msg.a / metadata.type / type / true / false / nil
  if (/^-?\d+(\.\d+)?$/.test(t)) return true; // 数字
  if (t.startsWith('"') && t.endsWith('"') && t.length >= 2) return true; // 字符串字面量
  return false;
}

// 在顶层（非引号内）查找运算符位置
function indexOfOp(s: string, op: string): number {
  let inStr = false;
  for (let i = 0; i <= s.length - op.length; i += 1) {
    const ch = s[i];
    if (ch === '"' && s[i - 1] !== '\\') inStr = !inStr;
    if (!inStr && s.slice(i, i + op.length) === op) {
      // >= 先于 > 匹配，== 先于 =；这里确保不命中 >= 的 '>' 之类
      const before = s[i - 1];
      const after = s[i + op.length];
      if ((op === '>' || op === '<') && after === '=') continue; // 已是 >= / <=
      if ((op === '>' || op === '<') && (before === '>' || before === '<')) continue;
      return i;
    }
  }
  return -1;
}

// 拆分函数参数（顶层逗号）
function splitArgs(s: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let inStr = false;
  let cur = '';
  for (let i = 0; i < s.length; i += 1) {
    const ch = s[i];
    if (ch === '"' && s[i - 1] !== '\\') inStr = !inStr;
    if (!inStr && ch === '(') depth += 1;
    if (!inStr && ch === ')') depth -= 1;
    if (!inStr && depth === 0 && ch === ',') {
      out.push(cur.trim());
      cur = '';
      continue;
    }
    cur += ch;
  }
  if (cur.trim() !== '') out.push(cur.trim());
  return out;
}

// 去掉字符串字面量引号并反转义；非字符串原样返回
function unquote(s: string): string {
  const t = s.trim();
  if (t.startsWith('"') && t.endsWith('"') && t.length >= 2) {
    return t.slice(1, -1).replace(/\\"/g, '"').replace(/\\\\/g, '\\');
  }
  return t;
}

// 顶层按 && / || 拆分，记录每个分隔符类型（支持混用）。
// ops[i] 是 parts[i] 与 parts[i+1] 之间的连接符；首条无条件符。
function splitTop(expr: string): { parts: string[]; ops: Combinator[] } | null {
  let depth = 0;
  let inStr = false;
  const ops: Array<{ idx: number; op: Combinator }> = [];
  for (let i = 0; i < expr.length - 1; i += 1) {
    const ch = expr[i];
    if (ch === '"' && expr[i - 1] !== '\\') inStr = !inStr;
    if (inStr) continue;
    if (ch === '(') depth += 1;
    else if (ch === ')') depth -= 1;
    else if (depth === 0 && ch === '&' && expr[i + 1] === '&') {
      ops.push({ idx: i, op: '&&' });
      i += 1;
    } else if (depth === 0 && ch === '|' && expr[i + 1] === '|') {
      ops.push({ idx: i, op: '||' });
      i += 1;
    }
  }
  if (ops.length === 0) return { parts: [expr], ops: [] };
  const parts: string[] = [];
  let last = 0;
  ops.forEach((o) => {
    parts.push(expr.slice(last, o.idx).trim());
    last = o.idx + 2;
  });
  parts.push(expr.slice(last).trim());
  return { parts, ops: ops.map((o) => o.op) };
}

// 解析完整表达式为规则行组；任一条无法解析则返回 null（调用方落入高级模式）。
// 返回规则带各自 join（与前一个条件的连接符，首条为 '&&' 占位、被忽略）。
// 支持混用 &&/||：先按顶层连接符切分；与组被整体括号包裹的段由 parseConjunction 剥层后二次拆分。
export function exprToRules(expr: string): { rules: CondRule[] } | null {
  const split = splitTop(expr.trim());
  if (!split) return null;
  const out: CondRule[] = [];
  for (let i = 0; i < split.parts.length; i += 1) {
    const seg = parseConjunction(split.parts[i]);
    if (!seg || seg.length === 0) return null;
    // 该段与前一段之间的连接符（顶层 op）；段内后续条件沿用各自 join。
    const segJoin: Combinator = i === 0 ? '&&' : split.ops[i - 1];
    seg.forEach((r, si) => out.push({ ...r, join: si === 0 ? segJoin : r.join ?? '&&' }));
  }
  return out.length > 0 ? { rules: out } : null;
}
