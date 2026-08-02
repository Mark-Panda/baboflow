import { useMemo, useRef } from 'react';

// 轻量代码编辑框：透明 textarea 叠加语法高亮层（零依赖），支持 JS/SQL/模板高亮、Tab 缩进、行号。
// 供节点配置区的脚本/表达式类字段使用；value/onChange 与 antd Form 受控协议一致。
export interface CodeFieldProps {
  value?: string;
  onChange?: (v: string) => void;
  language?: string;
  rows?: number;
  placeholder?: string;
}

const JS_KEYWORDS =
  'function|return|if|else|for|while|do|break|continue|switch|case|default|try|catch|finally|throw|new|var|let|const|typeof|instanceof|in|of|null|undefined|true|false|this|class|extends|super|yield|async|await|delete|void';
const SQL_KEYWORDS =
  'select|from|where|insert|update|delete|into|values|set|join|left|right|inner|outer|on|group|by|order|having|limit|offset|as|and|or|not|null|like|in|between|distinct|count|sum|avg|max|min|create|table|drop|alter';

function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// 逐行把代码转成带 <span> 高亮的 HTML。注释/字符串优先于关键字，避免误着色。
function highlightLine(line: string, lang: string): string {
  // 注释（// 或 --）整行变灰绿
  const commentIdx = (() => {
    const slash = line.indexOf('//');
    const dash = lang === 'sql' ? line.indexOf('--') : -1;
    if (slash === -1) return dash;
    if (dash === -1) return slash;
    return Math.min(slash, dash);
  })();
  let code = line;
  let comment = '';
  if (commentIdx >= 0) {
    code = line.slice(0, commentIdx);
    comment = line.slice(commentIdx);
  }

  // 字符串（单/双/反引号）
  let html = esc(code).replace(
    /('(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*"|`(?:[^`\\]|\\.)*`)/g,
    '<span class="cf-str">$1</span>'
  );
  // 数字
  html = html.replace(/\b(\d+\.?\d*)\b/g, '<span class="cf-num">$1</span>');
  // 关键字
  const kw = lang === 'sql' ? SQL_KEYWORDS : JS_KEYWORDS;
  html = html.replace(new RegExp(`\\b(${kw})\\b`, lang === 'sql' ? 'gi' : 'g'), '<span class="cf-kw">$1</span>');

  if (comment) html += `<span class="cf-cmt">${esc(comment)}</span>`;
  return html || ' ';
}

export default function CodeField({ value = '', onChange, language = 'javascript', rows = 6, placeholder }: CodeFieldProps) {
  const lang = String(language || 'javascript').toLowerCase();
  const taRef = useRef<HTMLTextAreaElement>(null);
  const preRef = useRef<HTMLPreElement>(null);
  const gutterRef = useRef<HTMLDivElement>(null);

  const highlighted = useMemo(() => {
    return value.split('\n').map((l) => highlightLine(l, lang)).join('\n');
  }, [value, lang]);

  const lineCount = value.split('\n').length;
  const lineNos = useMemo(() => Array.from({ length: Math.max(lineCount, rows) }, (_, i) => i + 1), [lineCount, rows]);

  // 输入层与高亮层/行号滚动同步
  const syncScroll = () => {
    const ta = taRef.current;
    if (preRef.current && ta) {
      preRef.current.scrollTop = ta.scrollTop;
      preRef.current.scrollLeft = ta.scrollLeft;
    }
    if (gutterRef.current && ta) gutterRef.current.scrollTop = ta.scrollTop;
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Tab') {
      e.preventDefault();
      const ta = e.currentTarget;
      const { selectionStart: s, selectionEnd: epos } = ta;
      const next = value.slice(0, s) + '  ' + value.slice(epos);
      onChange?.(next);
      requestAnimationFrame(() => {
        ta.selectionStart = ta.selectionEnd = s + 2;
      });
    }
  };

  const height = `${rows * 1.55 + 1}em`;

  return (
    <div className="bf-codefield" style={{ height }}>
      <div className="bf-codefield-gutter" ref={gutterRef} aria-hidden>
        {lineNos.map((n) => (
          <div key={n}>{n}</div>
        ))}
      </div>
      <div className="bf-codefield-body">
        <pre ref={preRef} className="bf-codefield-highlight" aria-hidden>
          <code dangerouslySetInnerHTML={{ __html: highlighted + '\n' }} />
        </pre>
        <textarea
          ref={taRef}
          className="bf-codefield-input"
          value={value}
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          placeholder={placeholder ?? (lang ? `// ${lang}` : '')}
          onChange={(e) => onChange?.(e.target.value)}
          onScroll={syncScroll}
          onKeyDown={onKeyDown}
          wrap="off"
        />
      </div>
    </div>
  );
}
