import { useEffect, useRef, useState } from "react";
import { basicSetup } from "codemirror";
import { EditorState } from "@codemirror/state";
import { indentWithTab } from "@codemirror/commands";
import { javascript } from "@codemirror/lang-javascript";
import { json } from "@codemirror/lang-json";
import { sql } from "@codemirror/lang-sql";
import { oneDark } from "@codemirror/theme-one-dark";
import { EditorView, keymap, placeholder } from "@codemirror/view";
import { Button, Modal, Tooltip, message } from "antd";
import {
  CompressOutlined,
  ExpandOutlined,
  FormatPainterOutlined,
} from "@ant-design/icons";
import "./codeField.css";

export interface CodeFieldProps {
  value?: string;
  onChange?: (v: string) => void;
  language?: string;
  rows?: number;
  placeholder?: string;
}

export async function formatCode(
  source: string,
  language: string,
): Promise<string> {
  const lang = String(language || "").toLowerCase();
  if (lang !== "javascript" && lang !== "js" && lang !== "json") {
    return source;
  }
  const [prettier, babelPlugin, estreePlugin] = await Promise.all([
    import("prettier/standalone"),
    import("prettier/plugins/babel"),
    import("prettier/plugins/estree"),
  ]);
  const parser = lang === "json" ? "json" : "babel";
  return prettier.format(source, {
    parser,
    plugins: [babelPlugin, estreePlugin],
    singleQuote: true,
    trailingComma: "all",
  });
}

function languageExtension(language: string) {
  const lang = String(language || "").toLowerCase();
  if (lang === "json") return json();
  if (lang === "sql") return sql();
  if (
    lang === "javascript" ||
    lang === "js" ||
    lang === "typescript" ||
    lang === "ts"
  ) {
    return javascript({ typescript: lang === "typescript" || lang === "ts" });
  }
  return [];
}

export default function CodeField({
  value = "",
  onChange,
  language = "javascript",
  rows = 6,
  placeholder: placeholderText,
}: CodeFieldProps) {
  const editorRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | undefined>(undefined);
  const editorValueRef = useRef(value);
  const onChangeRef = useRef(onChange);
  const formatSeq = useRef(0);
  const [formatting, setFormatting] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();
  const lang = String(language || "javascript").toLowerCase();
  const canFormat = lang === "javascript" || lang === "js" || lang === "json";

  onChangeRef.current = onChange;

  useEffect(() => {
    if (!editorRef.current) return undefined;
    const view = new EditorView({
      state: EditorState.create({
        doc: editorValueRef.current,
        extensions: [
          basicSetup,
          keymap.of([indentWithTab]),
          oneDark,
          languageExtension(lang),
          placeholder(placeholderText ?? `请输入${lang}内容`),
          EditorView.contentAttributes.of({
            "aria-label": placeholderText ?? `${lang.toUpperCase()} 配置编辑器`,
          }),
          EditorView.theme({
            "&": {
              minHeight: expanded
                ? "calc(72vh - 58px)"
                : `${rows * 1.55 + 1}em`,
            },
            ".cm-scroller": {
              overflow: "auto",
              fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
            },
            ".cm-content": {
              padding: "8px 10px",
              minHeight: expanded ? "calc(72vh - 70px)" : `${rows * 1.55}em`,
            },
            ".cm-gutters": { borderRight: "1px solid #1c2338" },
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) {
              const next = update.state.doc.toString();
              editorValueRef.current = next;
              onChangeRef.current?.(next);
            }
          }),
        ],
      }),
      parent: editorRef.current,
    });
    viewRef.current = view;
    return () => {
      editorValueRef.current = view.state.doc.toString();
      formatSeq.current += 1;
      setFormatting(false);
      view.destroy();
      viewRef.current = undefined;
    };
  }, [expanded, lang, rows, placeholderText]);

  useEffect(() => {
    editorValueRef.current = value;
    const view = viewRef.current;
    if (!view || value === view.state.doc.toString()) return;
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: value },
    });
  }, [value]);

  const format = async () => {
    if (!viewRef.current || !canFormat || formatting) return;
    const requestId = ++formatSeq.current;
    const view = viewRef.current;
    const source = view.state.doc.toString();
    setFormatting(true);
    try {
      const formatted = await formatCode(source, lang);
      const isCurrent =
        requestId === formatSeq.current &&
        viewRef.current === view &&
        view.state.doc.toString() === source;
      if (!isCurrent) {
        return;
      }
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: formatted },
      });
    } catch (err) {
      if (
        requestId === formatSeq.current &&
        viewRef.current === view &&
        view.state.doc.toString() === source
      ) {
        messageApi.error(
          `格式化失败：${err instanceof Error ? err.message : "代码格式不正确"}`,
        );
      }
    } finally {
      if (requestId === formatSeq.current) {
        setFormatting(false);
      }
    }
  };

  const toolbar = (
    <div className="bf-codefield-toolbar">
      <span>{lang.toUpperCase()} 编辑器</span>
      <div className="bf-codefield-actions">
        {canFormat && (
          <Tooltip title="格式化代码">
            <Button
              type="text"
              size="small"
              icon={<FormatPainterOutlined />}
              loading={formatting}
              aria-label="格式化代码"
              onClick={format}
            />
          </Tooltip>
        )}
        <Tooltip title={expanded ? "收起编辑器" : "展开编辑器"}>
          <Button
            type="text"
            size="small"
            icon={expanded ? <CompressOutlined /> : <ExpandOutlined />}
            aria-label={expanded ? "收起编辑器" : "展开编辑器"}
            onClick={() => setExpanded((open) => !open)}
          />
        </Tooltip>
      </div>
    </div>
  );
  const editorHost = <div className="bf-codefield-editor" ref={editorRef} />;

  return (
    <>
      {contextHolder}
      <div className="bf-codefield">
        {!expanded && toolbar}
        {!expanded && editorHost}
      </div>
      <Modal
        title={`${lang.toUpperCase()} 配置编辑器`}
        open={expanded}
        onCancel={() => setExpanded(false)}
        footer={null}
        width="min(1100px, calc(100vw - 48px))"
        centered
        destroyOnClose={false}
      >
        <div className="bf-codefield bf-codefield-expanded">
          {toolbar}
          {expanded && editorHost}
        </div>
      </Modal>
    </>
  );
}
